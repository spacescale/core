package controlplane

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/t0gun/spacescale/apps/scaled/internal/config"
	controlpb "github.com/t0gun/spacescale/packages/go/pb/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var errProtocolViolation = errors.New("control-plane protocol violation")

// Client owns dial/auth/register/heartbeat/reconnect behavior.
type Client struct {
	cfg    config.Config
	logger *slog.Logger
	seq    atomic.Uint64
}

func NewClient(cfg config.Config, logger *slog.Logger) *Client {
	if logger == nil {
		logger = slog.Default()
	}
	return &Client{
		cfg:    cfg,
		logger: logger,
	}
}

// Run starts the main lifecycle loop for the agent's communication with the control plane.
//
// It is a long-running "Infinite Loop" that ensures the agent stays connected.
// If the connection drops due to a network issue or a server restart, this
// function handles the "Exponential Backoff" strategy—waiting longer and longer
// between retries to avoid "hammering" a struggling server.
//
// The loop only exits if:
//  1. The provided context is canceled (graceful shutdown).
//  2. A "Permanent Error" occurs (like invalid authentication credentials).
func (c *Client) Run(ctx context.Context) error {
	// Initialize the first delay from configuration (e.g., 1s)
	backoffDelay := c.cfg.ControlPlane.ReconnectInitialBackoff // initial wait time before first retry
	for {
		// Try to establish and maintain a single gRPC session
		err := c.runSession(ctx)

		// Success or graceful shutdown: no need to retry, just exit.
		if err == nil || ctx.Err() != nil {
			return nil
		}

		// Security failure or bad config: stop immediately as retrying won't help.
		if isPermanentError(err) {
			return err
		}

		c.logger.Warn(
			"control-plane session ended; reconnecting",
			"error", err,
			"retry_in", backoffDelay.String(), // formats duration for logs, e.g. "10s"
		)

		// Create a timer to wait for the backoff duration
		timer := time.NewTimer(backoffDelay)

		select {
		case <-ctx.Done():
			// If the app shuts down WHILE we are waiting, stop the timer and exit.
			timer.Stop()
			return nil
		case <-timer.C:
			// The wait time is over! The loop will now continue and try runSession again.
		}

		// Increase the wait time for the NEXT failure (e.g., 1s -> 2s -> 4s...)
		// so we don't overwhelm the server if it's currently down.
		backoffDelay = nextBackoff(backoffDelay, c.cfg.ControlPlane.ReconnectMaxBackoff)
	}
}

func (c *Client) runSession(ctx context.Context) error {
	sessionCtx, cancelSession := context.WithCancel(ctx)
	defer cancelSession()

	dialCtx, cancelDial := context.WithTimeout(sessionCtx, c.cfg.ControlPlane.DialTimeout)
	defer cancelDial()

	conn, err := grpc.DialContext(
		dialCtx,
		c.cfg.ControlPlane.Addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithConnectParams(grpc.ConnectParams{
			Backoff: backoff.Config{
				BaseDelay:  1 * time.Second,
				Multiplier: 1.6,
				Jitter:     0.2,
				MaxDelay:   10 * time.Second,
			},
			MinConnectTimeout: c.cfg.ControlPlane.MinConnectTimeout,
		}),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                20 * time.Second,
			Timeout:             10 * time.Second,
			PermitWithoutStream: true,
		}),
	)
	if err != nil {
		return fmt.Errorf("dial control plane failed: %w", err)
	}
	defer conn.Close()

	client := controlpb.NewControlPlaneClient(conn)
	rpcCtx := metadata.AppendToOutgoingContext(sessionCtx, "authorization", "Bearer "+c.cfg.ControlPlane.AgentToken)

	healthCtx, cancelHealth := context.WithTimeout(rpcCtx, c.cfg.ControlPlane.HealthTimeout)
	healthResp, err := client.Health(healthCtx, &controlpb.HealthRequest{Caller: c.cfg.Agent.Caller, RequestId: c.nextRequestID("health")})
	cancelHealth()
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	if !healthResp.GetOk() {
		return fmt.Errorf("control plane not ready: %s", healthResp.GetStatus())
	}

	handshakeTimeout := c.cfg.ControlPlane.DialTimeout
	if handshakeTimeout < time.Second {
		handshakeTimeout = time.Second
	}

	stream, err := c.openSessionWithTimeout(rpcCtx, cancelSession, client, handshakeTimeout)
	if err != nil {
		return fmt.Errorf("open session failed: %w", err)
	}

	if err := stream.Send(&controlpb.AgentSessionRequest{
		RequestId: c.nextRequestID("register"),
		Event: &controlpb.AgentSessionRequest_Register{
			Register: &controlpb.RegisterRequest{
				Caller:           c.cfg.Agent.Caller,
				AgentKey:         c.cfg.Agent.Key,
				DesiredAgentName: c.cfg.Agent.Name,
				AgentVersion:     c.cfg.Agent.Version,
				Capabilities:     c.cfg.Agent.Capabilities,
				StartedUnix:      c.cfg.Agent.StartedUnix,
			},
		},
	}); err != nil {
		return fmt.Errorf("send register failed: %w", err)
	}

	firstResp, err := c.recvSessionResponseWithTimeout(sessionCtx, cancelSession, stream, handshakeTimeout)
	if err != nil {
		return fmt.Errorf("receive register response failed: %w", err)
	}

	reg := firstResp.GetRegister()
	if reg == nil {
		return protocolError("first response must be register")
	}
	if !reg.GetAccepted() {
		return fmt.Errorf("registration rejected: %s", reg.GetMessage())
	}
	if strings.TrimSpace(reg.GetSessionId()) == "" {
		return protocolError("register response missing session_id")
	}

	heartbeatEvery := time.Duration(reg.GetHeartbeatIntervalSeconds()) * time.Second
	if heartbeatEvery <= 0 {
		heartbeatEvery = c.cfg.ControlPlane.HeartbeatFallbackInterval
	}
	if heartbeatEvery < time.Second {
		heartbeatEvery = time.Second
	}

	c.logger.Info(
		"control-plane session established",
		"agent_id", reg.GetAgentId(),
		"session_id", reg.GetSessionId(),
		"heartbeat_every", heartbeatEvery.String(),
		"lease_ttl_seconds", reg.GetLeaseTtlSeconds(),
	)

	recvErrch := make(chan error, 1)
	go c.recvLoop(sessionCtx, stream, recvErrch)

	ticker := time.NewTicker(heartbeatEvery)
	defer ticker.Stop()

	for {
		select {
		case <-sessionCtx.Done():
			_ = stream.CloseSend()
			return nil
		case err := <-recvErrch:
			if errors.Is(err, io.EOF) {
				return fmt.Errorf("stream closed by control plane: %w", err)
			}
			return err
		case <-ticker.C:
			if err := stream.Send(&controlpb.AgentSessionRequest{
				RequestId: c.nextRequestID("heartbeat"),
				Event: &controlpb.AgentSessionRequest_Heartbeat{
					Heartbeat: &controlpb.HeartbeatRequest{
						SessionId:        reg.GetSessionId(),
						SentUnix:         time.Now().Unix(),
						Status:           c.cfg.Agent.Status,
						RunningWorkloads: 0,
						QueuedWorkloads:  0,
					},
				},
			}); err != nil {
				return fmt.Errorf("send heartbeat failed: %w", err)
			}
		}
	}
}

func (c *Client) openSessionWithTimeout(
	ctx context.Context,
	cancel context.CancelFunc,
	client controlpb.ControlPlaneClient,
	timeout time.Duration,
) (controlpb.ControlPlane_OpenSessionClient, error) {
	type result struct {
		stream controlpb.ControlPlane_OpenSessionClient
		err    error
	}

	resultCh := make(chan result, 1)
	go func() {
		stream, err := client.OpenSession(ctx)
		resultCh <- result{stream: stream, err: err}
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		cancel()
		return nil, fmt.Errorf("open session timeout after %s: %w", timeout, context.DeadlineExceeded)
	case res := <-resultCh:
		return res.stream, res.err
	}
}

func (c *Client) recvSessionResponseWithTimeout(
	ctx context.Context,
	cancel context.CancelFunc,
	stream controlpb.ControlPlane_OpenSessionClient,
	timeout time.Duration,
) (*controlpb.AgentSessionResponse, error) {
	type result struct {
		resp *controlpb.AgentSessionResponse
		err  error
	}

	resultCh := make(chan result, 1)
	go func() {
		resp, err := stream.Recv()
		resultCh <- result{resp: resp, err: err}
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		cancel()
		return nil, fmt.Errorf("register response timeout after %s: %w", timeout, context.DeadlineExceeded)
	case res := <-resultCh:
		return res.resp, res.err
	}
}

func (c *Client) recvLoop(ctx context.Context, stream controlpb.ControlPlane_OpenSessionClient, errCh chan<- error) {
	for {
		resp, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				select {
				case errCh <- io.EOF:
				default:
				}
				return
			}
			if ctx.Err() != nil || grpcCode(err) == codes.Canceled {
				return
			}
			select {
			case errCh <- fmt.Errorf("stream receive failed: %w", err):
			default:
			}
			return
		}

		switch {
		case resp.GetHeartbeat() != nil:
			hb := resp.GetHeartbeat()
			if !hb.GetOk() {
				c.logger.Warn(
					"heartbeat rejected by control plane",
					"server_unix", hb.GetServerUnix(),
					"message", hb.GetMessage(),
				)
			}

		case resp.GetDirective() != nil:
			d := resp.GetDirective()
			c.logger.Info(
				"directive received",
				"directive_id", d.GetDirectiveId(),
				"target", directiveTarget(d),
				"reason", d.GetReason(),
				"timeout_seconds", d.GetTimeoutSeconds(),
			)

		case resp.GetRegister() != nil:
			c.logger.Warn("unexpected register response on active stream")

		default:
			c.logger.Warn("received response with empty event payload")
		}
	}
}

func (c *Client) nextRequestID(prefix string) string {
	return fmt.Sprintf("%s-%d-%d", prefix, time.Now().UnixNano(), c.seq.Add(1))
}

func directiveTarget(d *controlpb.ControlDirective) string {
	switch {
	case d.GetNode() != nil:
		return "node"
	case d.GetAgent() != nil:
		return "agent"
	case d.GetWorkload() != nil:
		return "workload"
	default:
		return "unknown"
	}
}

func protocolError(message string) error {
	return fmt.Errorf("%w: %s", errProtocolViolation, message)
}

func nextBackoff(current, max time.Duration) time.Duration {
	next := current * 2
	if next > max {
		return max
	}
	return next
}

func isPermanentError(err error) bool {
	if errors.Is(err, errProtocolViolation) {
		return true
	}
	switch grpcCode(err) {
	case codes.Unauthenticated, codes.PermissionDenied, codes.InvalidArgument, codes.FailedPrecondition:
		return true
	default:
		return false
	}
}

func grpcCode(err error) codes.Code {
	for e := err; e != nil; e = errors.Unwrap(e) {
		if st, ok := status.FromError(e); ok {
			return st.Code()
		}
	}
	return codes.Unknown
}
