package workload

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/spacescale/core/scaled/node"
	"github.com/spacescale/core/shared/nats"
	pb "github.com/spacescale/core/shared/pb/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

type kvPutCall struct {
	ctx   context.Context
	key   string
	value []byte
}

type fakeKeyValue struct {
	puts    []kvPutCall
	putErr  error
	putHook func()
}

func (f *fakeKeyValue) Get(context.Context, string) (jetstream.KeyValueEntry, error) {
	return nil, nil
}

func (f *fakeKeyValue) GetRevision(context.Context, string, uint64) (jetstream.KeyValueEntry, error) {
	return nil, nil
}

func (f *fakeKeyValue) Put(ctx context.Context, key string, value []byte) (uint64, error) {
	f.puts = append(f.puts, kvPutCall{ctx: ctx, key: key, value: append([]byte(nil), value...)})
	if f.putHook != nil {
		f.putHook()
	}
	if f.putErr != nil {
		return 0, f.putErr
	}

	return uint64(len(f.puts)), nil
}

func (f *fakeKeyValue) PutString(context.Context, string, string) (uint64, error) {
	return 0, nil
}

func (f *fakeKeyValue) Create(context.Context, string, []byte, ...jetstream.KVCreateOpt) (uint64, error) {
	return 0, nil
}

func (f *fakeKeyValue) Update(context.Context, string, []byte, uint64) (uint64, error) {
	return 0, nil
}

func (f *fakeKeyValue) Delete(context.Context, string, ...jetstream.KVDeleteOpt) error {
	return nil
}

func (f *fakeKeyValue) Purge(context.Context, string, ...jetstream.KVDeleteOpt) error {
	return nil
}

func (f *fakeKeyValue) Watch(context.Context, string, ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	return nil, nil
}

func (f *fakeKeyValue) WatchAll(context.Context, ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	return nil, nil
}

func (f *fakeKeyValue) WatchFiltered(context.Context, []string, ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	return nil, nil
}

func (f *fakeKeyValue) Keys(context.Context, ...jetstream.WatchOpt) ([]string, error) {
	return nil, nil
}

func (f *fakeKeyValue) ListKeys(context.Context, ...jetstream.WatchOpt) (jetstream.KeyLister, error) {
	return nil, nil
}

func (f *fakeKeyValue) ListKeysFiltered(context.Context, ...string) (jetstream.KeyLister, error) {
	return nil, nil
}

func (f *fakeKeyValue) History(context.Context, string, ...jetstream.WatchOpt) ([]jetstream.KeyValueEntry, error) {
	return nil, nil
}

func (f *fakeKeyValue) Bucket() string {
	return "test-bucket"
}

func (f *fakeKeyValue) PurgeDeletes(context.Context, ...jetstream.KVPurgeOpt) error {
	return nil
}

func (f *fakeKeyValue) Status(context.Context) (jetstream.KeyValueStatus, error) {
	return nil, nil
}

func newTestWorkloadLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestNodeInfo() node.Info {
	return node.Info{
		Snapshot: node.Snapshot{
			BootID:     "boot-123",
			TotalCores: 8,
			TotalRAMMb: 65536,
		},
		Identity: node.Identity{
			NodeID: "node-123",
			Region: "us-east",
		},
	}
}

func requireNodeHeartbeat(t *testing.T, value []byte) *pb.NodeHeartbeat {
	t.Helper()

	var heartbeat pb.NodeHeartbeat
	require.NoError(t, proto.Unmarshal(value, &heartbeat))

	return &heartbeat
}

func TestPublishHeartbeatWritesNodeHeartbeat(t *testing.T) {
	info := newTestNodeInfo()
	kv := &fakeKeyValue{}

	publishHeartbeat(t.Context(), newTestWorkloadLogger(), info, kv, nats.NodeHeartbeatKey(info.Identity.NodeID), 7)

	require.Len(t, kv.puts, 1)
	require.Equal(t, nats.NodeHeartbeatKey(info.Identity.NodeID), kv.puts[0].key)

	heartbeat := requireNodeHeartbeat(t, kv.puts[0].value)
	require.Equal(t, "node-123", heartbeat.GetNodeId())
	require.Equal(t, uint64(7), heartbeat.GetSeqNo())
	require.Equal(t, "boot-123", heartbeat.GetBootId())
	require.NotZero(t, heartbeat.GetSentAtUnixNano())
}

func TestPublishHeartbeatSwallowsKVErrors(t *testing.T) {
	info := newTestNodeInfo()
	kv := &fakeKeyValue{putErr: errors.New("kv put failed")}

	publishHeartbeat(t.Context(), newTestWorkloadLogger(), info, kv, nats.NodeHeartbeatKey(info.Identity.NodeID), 1)

	require.Len(t, kv.puts, 1)
}

func TestRunHeartbeatPublishesInitialHeartbeatAndStopsOnCancel(t *testing.T) {
	info := newTestNodeInfo()
	ctx, cancel := context.WithCancel(t.Context())
	kv := &fakeKeyValue{
		putHook: cancel,
	}

	runHeartbeat(ctx, newTestWorkloadLogger(), info, kv)

	require.Len(t, kv.puts, 1)
	heartbeat := requireNodeHeartbeat(t, kv.puts[0].value)
	require.Equal(t, uint64(1), heartbeat.GetSeqNo())
	require.Equal(t, "node-123", heartbeat.GetNodeId())
}
