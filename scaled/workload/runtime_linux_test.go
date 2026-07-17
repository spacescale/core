package workload

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"github.com/spacescale/core/scaled/node"
	"github.com/spacescale/core/shared/nats"
	"github.com/spacescale/core/shared/pb/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func newTestWorkloadLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestNodeInfo() node.Info {
	return node.Info{
		Snapshot: node.Snapshot{
			BootID:       "boot-123",
			TotalCores:   8,
			TotalThreads: 16,
			TotalRAMMb:   65536,
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
	url := startTestNATSServer(t)
	client := newTestClient(t, url)
	kv, err := client.EnsureNodeHeartbeatKV(t.Context())
	require.NoError(t, err)

	info := newTestNodeInfo()
	publishHeartbeat(t.Context(), newTestWorkloadLogger(), info, kv, nats.NodeHeartbeatKey(info.Identity.NodeID), 7)

	entry, err := kv.Get(t.Context(), nats.NodeHeartbeatKey(info.Identity.NodeID))
	require.NoError(t, err)

	heartbeat := requireNodeHeartbeat(t, entry.Value())
	require.Equal(t, "node-123", heartbeat.GetNodeId())
	require.Equal(t, uint64(7), heartbeat.GetSeqNo())
	require.Equal(t, "boot-123", heartbeat.GetBootId())
	require.NotZero(t, heartbeat.GetSentAtUnixNano())
}

func TestPublishHeartbeatSwallowsKVErrors(t *testing.T) {
	url := startTestNATSServer(t)
	client := newTestClient(t, url)
	kv, err := client.EnsureNodeHeartbeatKV(t.Context())
	require.NoError(t, err)
	client.Close()

	info := newTestNodeInfo()
	require.NotPanics(t, func() {
		publishHeartbeat(t.Context(), newTestWorkloadLogger(), info, kv, nats.NodeHeartbeatKey(info.Identity.NodeID), 1)
	})
}

func TestRunHeartbeatPublishesInitialHeartbeatAndStopsOnCancel(t *testing.T) {
	url := startTestNATSServer(t)
	client := newTestClient(t, url)
	kv, err := client.EnsureNodeHeartbeatKV(t.Context())
	require.NoError(t, err)

	info := newTestNodeInfo()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	go func() {
		for {
			entry, err := kv.Get(ctx, nats.NodeHeartbeatKey(info.Identity.NodeID))
			if err == nil && entry != nil {
				cancel()
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	runHeartbeat(ctx, newTestWorkloadLogger(), info, kv)

	entry, err := kv.Get(t.Context(), nats.NodeHeartbeatKey(info.Identity.NodeID))
	require.NoError(t, err)
	heartbeat := requireNodeHeartbeat(t, entry.Value())
	require.Equal(t, uint64(1), heartbeat.GetSeqNo())
	require.Equal(t, "node-123", heartbeat.GetNodeId())
	require.Equal(t, "us-east", heartbeat.GetRegion())
}

func TestStartWiresHandlersAndHeartbeatWithRealNATS(t *testing.T) {
	url := startTestNATSServer(t)
	client := newTestClient(t, url)
	rawConn, err := natsgo.Connect(url)
	require.NoError(t, err)
	t.Cleanup(func() {
		rawConn.Close()
	})

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	info := newTestNodeInfo()
	info.RuntimePaths = node.RuntimePaths{
		FirecrackerPath: "/bin/true",
		JailerPath:      "/bin/true",
		KernelPath:      "/bin/true",
		RootFSPath:      "/bin/true",
	}

	require.NoError(t, Start(ctx, newTestWorkloadLogger(), info, client))

	kv, err := client.EnsureNodeHeartbeatKV(ctx)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		entry, err := kv.Get(ctx, nats.NodeHeartbeatKey(info.Identity.NodeID))
		return err == nil && entry != nil
	}, time.Second, 10*time.Millisecond)

	entry, err := kv.Get(ctx, nats.NodeHeartbeatKey(info.Identity.NodeID))
	require.NoError(t, err)

	var heartbeat pb.NodeHeartbeat
	require.NoError(t, proto.Unmarshal(entry.Value(), &heartbeat))
	require.Equal(t, info.Identity.NodeID, heartbeat.GetNodeId())
	require.Equal(t, info.Snapshot.BootID, heartbeat.GetBootId())
	require.Equal(t, uint64(1), heartbeat.GetSeqNo())
	require.NotZero(t, heartbeat.GetSentAtUnixNano())

	auctionReply := natsgo.NewInbox()
	auctionSub, err := rawConn.SubscribeSync(auctionReply)
	require.NoError(t, err)
	require.NoError(t, rawConn.Flush())
	require.NoError(t, rawConn.PublishRequest(nats.NodeAuctionSubject(info.Identity.Region), auctionReply, mustMarshalProto(t, &pb.AuctionRequest{
		MicrovmId: "vm-auction",
		Shape: &pb.MicroVMShape{
			Vcpu:    2,
			RamMb:   2048,
			CpuMode: pb.CpuMode_CPU_MODE_SHARED,
		},
	})))
	reply, err := auctionSub.NextMsg(2 * time.Second)
	require.NoError(t, err)

	var auctionResp pb.AuctionReply
	require.NoError(t, proto.Unmarshal(reply.Data, &auctionResp))
	require.Equal(t, info.Identity.NodeID, auctionResp.GetNodeId())
	require.Equal(t, info.Snapshot.BootID, auctionResp.GetBootId())

	launchReply := natsgo.NewInbox()
	launchSub, err := rawConn.SubscribeSync(launchReply)
	require.NoError(t, err)
	require.NoError(t, rawConn.Flush())
	require.NoError(t, rawConn.PublishRequest(nats.NodeMicroVMLaunchSubject(info.Snapshot.BootID), launchReply, mustMarshalProto(t, &pb.MicroVMLaunchRequest{
		MicrovmId: "vm-launch",
		Shape: &pb.MicroVMShape{
			Vcpu:    2,
			RamMb:   2048,
			CpuMode: pb.CpuMode_CPU_MODE_SHARED,
		},
	})))
	launchMsg, err := launchSub.NextMsg(2 * time.Second)
	require.NoError(t, err)

	var launchResp pb.MicroVMLaunchResponse
	require.NoError(t, proto.Unmarshal(launchMsg.Data, &launchResp))
	require.False(t, launchResp.GetAccepted())
	require.Equal(t, "reservation expired or not found", launchResp.GetErrorMessage())
}

func mustMarshalProto(t *testing.T, msg proto.Message) []byte {
	t.Helper()
	data, err := proto.Marshal(msg)
	require.NoError(t, err)
	return data
}
