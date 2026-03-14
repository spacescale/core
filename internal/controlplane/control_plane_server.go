package controlplane

import (
	"context"
	"crypto/subtle"
	"errors"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/t0gun/spacescale/internal/controlplane/state"
	controlpb "github.com/t0gun/spacescale/packages/go/pb/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// ControlPlaneConfig defines the configuration parameters for initializing the ControlPlaneServer.
//
//goland:noinspection ALL
type ControlPlaneConfig struct {
	AgentToken        string
	HeartbeatInterval time.Duration
	LeaseTTL          time.Duration
	Version           string
	Logger            *slog.Logger
}

// ControlPlaneServer is the core gRPC server that manages agent life-cycles,
// health checks (heartbeats), and communication via OpenSession streams.
//
//goland:noinspection ALL
type ControlPlaneServer struct {
	controlpb.UnimplementedControlPlaneServer
	agents            *state.PersistingAgentStore
	agentToken        string
	heartbeatInterval time.Duration
	leaseTTL          time.Duration // How long control plane waits before declaring an agent dead
	version           string
	startedUnix       int64
	logger            *slog.Logger
}

// NewControlPlaneServer creates and initializes a ControlPlaneServer with the provided
// configuration. It ensures that the agent store and agent token are valid, and sets
// reasonable defaults for HeartbeatInterval and LeaseTTL if they are not specified.
func NewControlPlaneServer(agents *state.PersistingAgentStore, cfg ControlPlaneConfig) (*ControlPlaneServer, error) {
	if agents == nil {
		return nil, errors.New("control plane requires non-nil agent store")
	}
	token := strings.TrimSpace(cfg.AgentToken)
	if token == "" {
		return nil, errors.New("control plane requires non-empty agent token")
	}
	if cfg.HeartbeatInterval <= 0 {
		cfg.HeartbeatInterval = 10 * time.Second // if heartbeat interval time not configured set default to 10 seconds
	}
	if cfg.LeaseTTL < cfg.HeartbeatInterval*2 {
		cfg.LeaseTTL = cfg.HeartbeatInterval * 3 // make timeout window longer so missed heartbeats can come in twice before declaring  agent dead
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &ControlPlaneServer{
		agents:            agents,
		agentToken:        token,
		heartbeatInterval: cfg.HeartbeatInterval,
		leaseTTL:          cfg.LeaseTTL,
		version:           strings.TrimSpace(cfg.Version),
		startedUnix:       time.Now().Unix(),
		logger:            cfg.Logger,
	}, nil
}

// UnaryAuthInterceptor returns a gRPC interceptor for standard "Unary" RPCs.
//
// A "Unary" RPC is the simplest communication pattern in gRPC, where the client
// sends a single request and the server returns a single response (similar to a
// standard REST API call).
//
// This interceptor acts as a security gatekeeper:
//  1. It intercepts every incoming single-request call before it reaches the handler.
//  2. It calls validateAuth to check for valid credentials in the context.
//  3. If authentication fails, it short-circuits the request and returns an error.
//  4. If successful, it allows the request to proceed to the actual service method.
func (s *ControlPlaneServer) UnaryAuthInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if err := s.validateAuth(ctx); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

// StreamAuthInterceptor returns a gRPC interceptor for "Streaming" RPCs.
//
// A "Streaming" RPC is a long-lived connection where either the client, the
// server, or both can send a continuous flow of multiple messages over time.
//
// Unlike the Unary interceptor, which runs for every single request, this
// interceptor runs exactly ONCE at the very beginning of the stream's lifecycle.
// If the initial connection is authenticated successfully, the stream remains
// open for data transfer without re-authenticating every individual message.
func (s *ControlPlaneServer) StreamAuthInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, stream grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if err := s.validateAuth(stream.Context()); err != nil {
			return err
		}
		return handler(srv, stream)
	}
}

// NewGRPCServer initializes a new gRPC server with the ControlPlaneServer handler,
// setting maximum message sizes and attaching the authentication interceptors.
func NewGRPCServer(handler *ControlPlaneServer, maxRecvBytes, maxSendBytes int) *grpc.Server {
	if maxRecvBytes <= 0 {
		maxRecvBytes = 1 << 20
	}
	if maxSendBytes <= 0 {
		maxSendBytes = 1 << 20
	}
	srv := grpc.NewServer(
		grpc.MaxRecvMsgSize(maxRecvBytes),
		grpc.MaxSendMsgSize(maxSendBytes),
		grpc.UnaryInterceptor(handler.UnaryAuthInterceptor()),
		grpc.StreamInterceptor(handler.StreamAuthInterceptor()),
	)
	controlpb.RegisterControlPlaneServer(srv, handler)
	return srv
}

// Health implements the ControlPlane.Health gRPC method.
//
// It provides a simple readiness probe for clients (like agents) to verify
// the control plane is reachable before attempting more complex operations.
// The underscores in the parameters (_ context.Context, _ *controlpb.HealthRequest)
// allow the method to satisfy the gRPC interface requirement while explicitly
// ignoring the input data, as it is not needed for this simple "OK" response.
func (s *ControlPlaneServer) Health(_ context.Context, _ *controlpb.HealthRequest) (*controlpb.HealthResponse, error) {
	return &controlpb.HealthResponse{
		Ok:          true,
		Status:      "ready",
		StartedUnix: s.startedUnix,
		Message:     "ok",
	}, nil
}

func (s *ControlPlaneServer) OpenSession(stream controlpb.ControlPlane_OpenSessionServer) error {
	first, err := stream.Recv()
	if err != nil {
		return err
	}
	reg := first.GetRegister()
	if reg == nil {
		return status.Error(codes.InvalidArgument, "first message must be register")
	}
	agentKey := strings.TrimSpace(reg.GetAgentKey())
	if agentKey == "" {
		return status.Error(codes.InvalidArgument, "register.agent_key is required")
	}

	sessionID := uuid.NewString()
	now := time.Now().UTC()

	s.agents.Register(
		agentKey,
		sessionID,
		reg.GetDesiredAgentName(),
		reg.GetAgentVersion(),
		reg.GetCapabilities(),
		now,
	)
	if streamErr := stream.Send(&controlpb.AgentSessionResponse{
		RequestId: first.GetRequestId(),
		Event: &controlpb.AgentSessionResponse_Register{
			Register: &controlpb.RegisterResponse{
				Accepted:                 true,
				AgentId:                  agentKey,
				SessionId:                sessionID,
				HeartbeatIntervalSeconds: int64(s.heartbeatInterval / time.Second), // need in64 time in seconds
				LeaseTtlSeconds:          int64(s.leaseTTL / time.Second),          // that's why cos grpc uses it. go time.seconds uses float64
				Message:                  "registered",
			},
		},
	}); streamErr != nil {
		return streamErr
	}
	s.logger.Info("agent registered", "agent_id", agentKey, "session_id", sessionID)

	// the cleanup action for the grpc stream
	// if an agent crashes or if there is an error mark offline tells agent manager connection is gone
	defer func() {
		s.agents.MarkOffline(agentKey, sessionID, time.Now().UTC())
		s.logger.Info("agent disconnected", "agent_id", agentKey, "session_id", sessionID)
	}()
	// we want open session to be a persistent connection. we open down it a for loop
	for {
		req, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			// if stream reaches end of input
			return nil
		}
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}

		// get agent heartbeat
		hb := req.GetHeartbeat()
		if hb == nil {
			// ignore non-heartbeat events for now
			continue
		}
		// check sessions at every message
		if strings.TrimSpace(hb.GetSessionId()) != sessionID {
			return status.Error(codes.PermissionDenied, "session id mismatch")
		}

		// once heartbeat and session is valid we can store the other info alongside to the agent manager
		if err := s.agents.UpdateHeartbeat(
			agentKey,
			sessionID,
			hb.GetStatus(),
			hb.GetRunningWorkloads(),
			hb.GetQueuedWorkloads(),
			time.Now().UTC(),
		); err != nil {
			return status.Error(codes.PermissionDenied, err.Error())
		}

		if err := stream.Send(&controlpb.AgentSessionResponse{
			RequestId: req.GetRequestId(),
			Event: &controlpb.AgentSessionResponse_Heartbeat{
				Heartbeat: &controlpb.HeartbeatResponse{
					Ok:         true,
					ServerUnix: time.Now().Unix(),
					Message:    "heartbeat accepted",
				},
			},
		}); err != nil {
			return err
		}
	}
}

// validateAuth checks the incoming context for a valid "Bearer" token.
//
// It retrieves the "authorization" metadata from the context, extracts the
// token using parseBearer, and compares it against the server's configured
// agent token. To prevent timing attacks, it uses subtle.ConstantTimeCompare
// for the final verification. If any step fails, it returns a gRPC
// Unauthenticated error.
func (s *ControlPlaneServer) validateAuth(ctx context.Context) error {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		s.logger.Warn("auth failed", "reason", "missing_metadata")
		return status.Error(codes.Unauthenticated, "missing metadata")
	}
	values := md.Get("authorization")
	if len(values) == 0 {
		s.logger.Warn("auth failed", "reason", "missing_authorization_header")
		return status.Error(codes.Unauthenticated, "missing authorization header")
	}
	token := parseBearer(values[0])
	if token == "" {
		s.logger.Warn("auth failed", "reason", "invalid_bearer_format")
		return status.Error(codes.Unauthenticated, "invalid authorization header")
	}

	if subtle.ConstantTimeCompare([]byte(token), []byte(s.agentToken)) != 1 {
		s.logger.Warn("auth failed", "reason", "token_mismatch")
		return status.Error(codes.Unauthenticated, "invalid token")
	}

	return nil
}

// parseBearer extracts the token from a Bearer authentication string.
//
// It expects the input to follow the "Bearer <token>" format commonly found
// in Authorization headers. The function is case-insensitive and handles
// additional whitespace around the "Bearer" prefix and the token itself.
// If the input does not match the Bearer scheme, it returns an empty string.
func parseBearer(raw string) string {
	v := strings.TrimSpace(raw)
	if len(v) < 7 || !strings.EqualFold(v[:7], "Bearer ") {
		return ""
	}
	return strings.TrimSpace(v[7:])
}
