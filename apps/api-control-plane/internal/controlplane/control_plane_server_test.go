package controlplane

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/t0gun/spacescale/internal/controlplane/state"
	controlv1 "github.com/t0gun/spacescale/packages/proto-go/control/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const (
	testAgentToken = "test-control-plane-token"
	testBufSize    = 1024 * 1024
)

func TestControlPlaneHealthRejectsMalformedBearer(t *testing.T) {
	client := newBufconnControlPlaneClient(t)
	ctx, cancel := authCtx(t, "Bearer"+testAgentToken)
	defer cancel()

	_, err := client.Health(ctx, &controlv1.HealthRequest{Caller: "scaled/test"})
	requireGRPCCode(t, err, codes.Unauthenticated)
}

func TestControlPlaneOpenSessionRequiresRegisterFirst(t *testing.T) {
	client := newBufconnControlPlaneClient(t)
	ctx, cancel := authCtx(t, "Bearer "+testAgentToken)
	defer cancel()

	stream, err := client.OpenSession(ctx)
	require.NoError(t, err)
	defer func() { _ = stream.CloseSend() }()

	err = stream.Send(heartbeatReq("hb-1", "no-session"))
	require.NoError(t, err)

	_, err = stream.Recv()
	requireGRPCCode(t, err, codes.InvalidArgument)
}

func TestControlPlaneOpenSessionRegisterAndHeartbeatSuccess(t *testing.T) {
	client := newBufconnControlPlaneClient(t)
	ctx, cancel := authCtx(t, "Bearer "+testAgentToken)
	defer cancel()

	stream, err := client.OpenSession(ctx)
	require.NoError(t, err)
	defer func() { _ = stream.CloseSend() }()

	err = stream.Send(registerReq())
	require.NoError(t, err)

	first, err := stream.Recv()
	require.NoError(t, err)

	reg := first.GetRegister()
	require.NotNil(t, reg)
	require.True(t, reg.GetAccepted())
	require.NotEmpty(t, reg.GetSessionId())
	require.Equal(t, "reg-1", first.GetRequestId())

	err = stream.Send(heartbeatReq("hb-1", reg.GetSessionId()))
	require.NoError(t, err)

	ack, err := stream.Recv()
	require.NoError(t, err)
	require.Equal(t, "hb-1", ack.GetRequestId())

	hb := ack.GetHeartbeat()
	require.NotNil(t, hb)
	require.True(t, hb.GetOk())
	require.NotZero(t, hb.GetServerUnix())
}

func TestControlPlaneOpenSessionRejectsSessionMismatch(t *testing.T) {
	client := newBufconnControlPlaneClient(t)
	ctx, cancel := authCtx(t, "Bearer "+testAgentToken)
	defer cancel()

	stream, err := client.OpenSession(ctx)
	require.NoError(t, err)
	defer func() { _ = stream.CloseSend() }()

	err = stream.Send(registerReq())
	require.NoError(t, err)

	first, err := stream.Recv()
	require.NoError(t, err)
	require.NotNil(t, first.GetRegister())

	err = stream.Send(heartbeatReq("hb-mismatch", "wrong-session-id"))
	require.NoError(t, err)

	_, err = stream.Recv()
	requireGRPCCode(t, err, codes.PermissionDenied)
}

func newBufconnControlPlaneClient(t *testing.T) controlv1.ControlPlaneClient {
	t.Helper()

	handler, err := NewControlPlaneServer(state.NewTransientAgentStore(30*time.Second, nil), ControlPlaneConfig{
		AgentToken:        testAgentToken,
		HeartbeatInterval: 10 * time.Second,
		LeaseTTL:          45 * time.Second,
		Version:           "test",
	})
	require.NoError(t, err)

	server := NewGRPCServer(handler, 1<<20, 1<<20)
	listener := bufconn.Listen(testBufSize)

	go func() {
		_ = server.Serve(listener)
	}()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelDial()

	conn, err := grpc.DialContext(
		dialCtx,
		"bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = conn.Close()
		server.Stop()
		_ = listener.Close()
	})

	return controlv1.NewControlPlaneClient(conn)
}

func authCtx(t *testing.T, authHeader string) (context.Context, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	return metadata.AppendToOutgoingContext(ctx, "authorization", authHeader), cancel
}

func registerReq() *controlv1.AgentSessionRequest {
	return &controlv1.AgentSessionRequest{
		RequestId: "reg-1",
		Event: &controlv1.AgentSessionRequest_Register{
			Register: &controlv1.RegisterRequest{
				Caller:           "scaled/test",
				AgentKey:         "agent-key-1",
				DesiredAgentName: "agent-test",
				AgentVersion:     "vtest",
				Capabilities:     []string{"heartbeat"},
				StartedUnix:      time.Now().Unix(),
			},
		},
	}
}

func heartbeatReq(requestID, sessionID string) *controlv1.AgentSessionRequest {
	return &controlv1.AgentSessionRequest{
		RequestId: requestID,
		Event: &controlv1.AgentSessionRequest_Heartbeat{
			Heartbeat: &controlv1.HeartbeatRequest{
				SessionId:        sessionID,
				SentUnix:         time.Now().Unix(),
				Status:           "ready",
				RunningWorkloads: 1,
				QueuedWorkloads:  0,
			},
		},
	}
}

func requireGRPCCode(t *testing.T, err error, expected codes.Code) {
	t.Helper()
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok, "expected gRPC status error, got %T (%v)", err, err)
	require.Equal(t, expected, st.Code())
}
