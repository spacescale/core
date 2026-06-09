package nats

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/spacescale/core/shared/pb/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestJetStream(t *testing.T) {
	url := startTestNATSServer(t)
	client := newTestClient(t, url)

	js, err := client.JetStream()
	require.NoError(t, err)
	require.NotNil(t, js)
}

func TestEnsureKeyValue(t *testing.T) {
	url := startTestNATSServer(t)
	client := newTestClient(t, url)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cfg := jetstream.KeyValueConfig{
		Bucket: "TEST_BUCKET",
	}

	// First call should create the bucket
	kv, err := client.EnsureKeyValue(ctx, cfg)
	require.NoError(t, err)
	require.Equal(t, "TEST_BUCKET", kv.Bucket())

	// Second call should handle idempotency safely
	kv2, err := client.EnsureKeyValue(ctx, cfg)
	require.NoError(t, err)
	require.Equal(t, "TEST_BUCKET", kv2.Bucket())
}

func TestEnsureKeyValueReturnsErrorOnClosedConnection(t *testing.T) {
	url := startTestNATSServer(t)
	client := newTestClient(t, url)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Close connection so CreateOrUpdateKeyValue fails
	client.Close()

	cfg := jetstream.KeyValueConfig{Bucket: "TEST_CLOSED"}
	_, err := client.EnsureKeyValue(ctx, cfg)
	require.Error(t, err)
}

func TestEnsureNodeHeartbeatKV(t *testing.T) {
	url := startTestNATSServer(t)
	client := newTestClient(t, url)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	kv, err := client.EnsureNodeHeartbeatKV(ctx)
	require.NoError(t, err)
	require.Equal(t, NodeHeartbeatBucket, kv.Bucket())

	status, err := kv.Status(ctx)
	require.NoError(t, err)
	require.Equal(t, NodeHeartbeatTTL, status.TTL())
}

func TestNodeHeartbeatKey(t *testing.T) {
	require.Equal(t, "nodes.node-123", NodeHeartbeatKey("node-123"))
}

func TestNodeHeartbeatWatchAll(t *testing.T) {
	require.Equal(t, "nodes.>", NodeHeartbeatWatchAll())
}

func TestPutProtoKV(t *testing.T) {
	url := startTestNATSServer(t)
	client := newTestClient(t, url)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	kv, err := client.EnsureNodeHeartbeatKV(ctx)
	require.NoError(t, err)

	want := &pb.MicroVMLaunchResponse{
		Accepted:     true,
		ErrorMessage: "test-heartbeat",
	}

	key := NodeHeartbeatKey("node-test")
	seq, err := PutProtoKV(ctx, kv, key, want)
	require.NoError(t, err)
	require.Greater(t, seq, uint64(0))

	entry, err := kv.Get(ctx, key)
	require.NoError(t, err)

	var got pb.MicroVMLaunchResponse
	require.NoError(t, proto.Unmarshal(entry.Value(), &got))
	require.True(t, got.GetAccepted())
	require.Equal(t, "test-heartbeat", got.GetErrorMessage())
}

func TestPutProtoKVReturnsErrorForMarshalFailure(t *testing.T) {
	url := startTestNATSServer(t)
	client := newTestClient(t, url)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	kv, err := client.EnsureNodeHeartbeatKV(ctx)
	require.NoError(t, err)

	// Re-using the failingProtoMessage from client_test.go
	seq, err := PutProtoKV(ctx, kv, "test.key", failingProtoMessage{})
	require.Error(t, err)
	require.Equal(t, uint64(0), seq)
}
