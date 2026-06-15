package workload

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/spacescale/core/scaled/workload/microvm"
	"github.com/spacescale/core/shared/nats"
	"github.com/spacescale/core/shared/pb/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

type fakeLauncher struct {
	launchCalled bool
	launchCtx    context.Context
	launchReq    microvm.LaunchRequest
	launchVM     *microvm.ActiveVM
	launchErr    error

	stopCalled bool
	stopCtx    context.Context
	stopID     string
	stopErr    error
}

func (f *fakeLauncher) Launch(ctx context.Context, req microvm.LaunchRequest) (*microvm.ActiveVM, error) {
	f.launchCalled = true
	f.launchCtx = ctx
	f.launchReq = req
	return f.launchVM, f.launchErr
}

func (f *fakeLauncher) Stop(ctx context.Context, microvmID string) error {
	f.stopCalled = true
	f.stopCtx = ctx
	f.stopID = microvmID
	return f.stopErr
}

func newTestLaunchHandler(t *testing.T, launcher launcher) *launchHandler {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	capacity := NewCapacity(65536, 8)
	handler := newLaunchHandler(log, capacity, "boot-123", launcher)
	handler.resolveImageConfig = func(context.Context, string) (resolvedOCIConfig, error) {
		return resolvedOCIConfig{
			ImageDigest: "sha256:test",
			Entrypoint:  []string{"app"},
			Cmd:         []string{"serve"},
		}, nil
	}

	return handler
}

func TestLaunchHandlerHandleRejectedMissingReplySubject(t *testing.T) {
	launcher := &fakeLauncher{}
	handler := newTestLaunchHandler(t, launcher)

	msg := launchMsg(t, "", &pb.MicroVMLaunchRequest{
		MicrovmId: "vm-1",
		Shape: &pb.MicroVMShape{
			Vcpu:    2,
			RamMb:   2048,
			CpuMode: pb.CpuMode_CPU_MODE_SHARED,
		},
	})

	err := handler.handle(context.Background(), newTestClient(t, startTestNATSServer(t)), msg)
	require.ErrorContains(t, err, "missing reply subject")
	require.False(t, launcher.launchCalled)
}

func TestLaunchHandlerHandleRejectsInvalidProto(t *testing.T) {
	launcher := &fakeLauncher{}
	handler := newTestLaunchHandler(t, launcher)

	msg := &nats.Msg{
		Reply: "reply.subject",
		Data:  []byte("not-proto"),
	}

	err := handler.handle(context.Background(), newTestClient(t, startTestNATSServer(t)), msg)
	require.ErrorContains(t, err, "unmarshal proto")
	require.False(t, launcher.launchCalled)
}

func TestLaunchHandlerHandleRejectsMissingMicroVMID(t *testing.T) {
	launcher := &fakeLauncher{}
	handler := newTestLaunchHandler(t, launcher)

	msg := launchMsg(t, "reply.subject", &pb.MicroVMLaunchRequest{
		Shape: &pb.MicroVMShape{
			Vcpu:    2,
			RamMb:   2048,
			CpuMode: pb.CpuMode_CPU_MODE_SHARED,
		},
	})

	err := handler.handle(context.Background(), newTestClient(t, startTestNATSServer(t)), msg)
	require.ErrorContains(t, err, "missing microvm id")
	require.False(t, launcher.launchCalled)
}

func TestLaunchHandlerHandleRejectsInvalidShape(t *testing.T) {
	launcher := &fakeLauncher{}
	handler := newTestLaunchHandler(t, launcher)

	msg := launchMsg(t, "reply.subject", &pb.MicroVMLaunchRequest{
		MicrovmId: "vm-1",
		Shape: &pb.MicroVMShape{
			Vcpu:  2,
			RamMb: 2048,
		},
	})

	err := handler.handle(context.Background(), newTestClient(t, startTestNATSServer(t)), msg)
	require.ErrorIs(t, err, ErrInvalidMicroVMShape)
	require.False(t, launcher.launchCalled)
}

func TestLaunchHandlerHandleRejectsMissingReservation(t *testing.T) {
	launcher := &fakeLauncher{}
	handler := newTestLaunchHandler(t, launcher)
	client := newTestClient(t, startTestNATSServer(t))
	replies := capturePublishedMsg(t, client)

	msg := launchMsg(t, "reply.subject", &pb.MicroVMLaunchRequest{
		MicrovmId: "vm-1",
		Shape: &pb.MicroVMShape{
			Vcpu:    2,
			RamMb:   2048,
			CpuMode: pb.CpuMode_CPU_MODE_SHARED,
		},
	})

	err := handler.handle(context.Background(), client, msg)
	require.NoError(t, err)
	require.False(t, launcher.launchCalled)

	reply := requireLaunchReply(t, receivePublishedMsg(t, replies))
	require.False(t, reply.GetAccepted())
	require.Equal(t, "reservation expired or not found", reply.GetErrorMessage())
}

func TestLaunchHandlerHandleRejectsOCIConfigResolutionFailure(t *testing.T) {
	resolverErr := errors.New("resolve failed")
	launcher := &fakeLauncher{}
	handler := newTestLaunchHandler(t, launcher)
	handler.resolveImageConfig = func(context.Context, string) (resolvedOCIConfig, error) {
		return resolvedOCIConfig{}, resolverErr
	}
	client := newTestClient(t, startTestNATSServer(t))
	replies := capturePublishedMsg(t, client)

	_, ok := handler.capacity.Reserve("vm-1", HardwareSpec{
		VCPU: 2,
		RAM:  2048,
	}, time.Second)
	require.True(t, ok)

	msg := launchMsg(t, "reply.subject", &pb.MicroVMLaunchRequest{
		MicrovmId: "vm-1",
		ImageRef:  "ghcr.io/acme/app:latest",
		Shape: &pb.MicroVMShape{
			Vcpu:    2,
			RamMb:   2048,
			CpuMode: pb.CpuMode_CPU_MODE_SHARED,
		},
	})

	err := handler.handle(context.Background(), client, msg)
	require.ErrorIs(t, err, resolverErr)
	require.False(t, launcher.launchCalled)

	reply := requireLaunchReply(t, receivePublishedMsg(t, replies))
	require.False(t, reply.GetAccepted())
	require.Equal(t, "resolve failed", reply.GetErrorMessage())
}

func TestLaunchHandlerHandleRevertsCapacityWhenLaunchFails(t *testing.T) {
	launchErr := errors.New("launch failed")
	launcher := &fakeLauncher{launchErr: launchErr}
	handler := newTestLaunchHandler(t, launcher)
	client := newTestClient(t, startTestNATSServer(t))
	replies := capturePublishedMsg(t, client)

	_, ok := handler.capacity.Reserve("vm-1", HardwareSpec{
		VCPU: 2,
		RAM:  2048,
	}, time.Second)
	require.True(t, ok)

	msg := launchMsg(t, "reply.subject", &pb.MicroVMLaunchRequest{
		MicrovmId: "vm-1",
		ImageRef:  "ghcr.io/acme/app:latest",
		Env: map[string]string{
			"FOO": "bar",
		},
		RuntimePort: 9090,
		Shape: &pb.MicroVMShape{
			Vcpu:    2,
			RamMb:   2048,
			CpuMode: pb.CpuMode_CPU_MODE_SHARED,
		},
	})

	err := handler.handle(context.Background(), client, msg)
	require.ErrorIs(t, err, launchErr)

	require.True(t, launcher.launchCalled)
	require.Equal(t, "vm-1", launcher.launchReq.MicroVMID)
	require.Equal(t, uint32(2), launcher.launchReq.VCPU)
	require.Equal(t, uint64(2048), launcher.launchReq.RAMMB)
	require.Equal(t, "ghcr.io/acme/app:latest", launcher.launchReq.ImageRef)
	require.Equal(t, "sha256:test", launcher.launchReq.ImageDigest)
	require.Equal(t, []string{"app", "serve"}, launcher.launchReq.Command)
	require.Equal(t, map[string]string{"FOO": "bar"}, launcher.launchReq.Env)
	require.Equal(t, uint32(9090), launcher.launchReq.RuntimePort)
	require.False(t, launcher.stopCalled)

	reply := requireLaunchReply(t, receivePublishedMsg(t, replies))
	require.False(t, reply.GetAccepted())
	require.Equal(t, "launch failed", reply.GetErrorMessage())

	require.Zero(t, handler.capacity.usedRAMMB)
	require.Zero(t, handler.capacity.usedSharedVCPU)
	require.Zero(t, handler.capacity.reservedRAMMB)
	require.Zero(t, handler.capacity.reservedSharedVCPU)
}

func TestLaunchHandlerHandlePublishesAcceptedAfterLaunch(t *testing.T) {
	launcher := &fakeLauncher{
		launchVM: &microvm.ActiveVM{MicroVMID: "vm-1"},
	}
	handler := newTestLaunchHandler(t, launcher)
	client := newTestClient(t, startTestNATSServer(t))
	replies := capturePublishedMsg(t, client)

	_, ok := handler.capacity.Reserve("vm-1", HardwareSpec{
		VCPU: 2,
		RAM:  2048,
	}, time.Second)
	require.True(t, ok)

	msg := launchMsg(t, "reply.subject", &pb.MicroVMLaunchRequest{
		MicrovmId: "vm-1",
		Shape: &pb.MicroVMShape{
			Vcpu:    2,
			RamMb:   2048,
			CpuMode: pb.CpuMode_CPU_MODE_SHARED,
		},
	})

	err := handler.handle(context.Background(), client, msg)
	require.NoError(t, err)

	require.True(t, launcher.launchCalled)
	require.False(t, launcher.stopCalled)

	reply := requireLaunchReply(t, receivePublishedMsg(t, replies))
	require.True(t, reply.GetAccepted())
	require.Empty(t, reply.GetErrorMessage())

	require.Equal(t, uint64(2048), handler.capacity.usedRAMMB)
	require.Equal(t, uint32(2), handler.capacity.usedSharedVCPU)
	require.Zero(t, handler.capacity.reservedRAMMB)
	require.Zero(t, handler.capacity.reservedSharedVCPU)
}

func TestLaunchHandlerHandleStopsVMAndRevertsCapacityWhenPublishFails(t *testing.T) {
	launcher := &fakeLauncher{
		launchVM: &microvm.ActiveVM{MicroVMID: "vm-1"},
	}
	handler := newTestLaunchHandler(t, launcher)
	client := newTestClient(t, startTestNATSServer(t))
	client.Close()

	_, ok := handler.capacity.Reserve("vm-1", HardwareSpec{
		VCPU: 2,
		RAM:  2048,
	}, time.Second)
	require.True(t, ok)

	msg := launchMsg(t, "reply.subject", &pb.MicroVMLaunchRequest{
		MicrovmId: "vm-1",
		Shape: &pb.MicroVMShape{
			Vcpu:    2,
			RamMb:   2048,
			CpuMode: pb.CpuMode_CPU_MODE_SHARED,
		},
	})

	err := handler.handle(context.Background(), client, msg)
	require.Error(t, err)

	require.True(t, launcher.launchCalled)
	require.True(t, launcher.stopCalled)
	require.Equal(t, "vm-1", launcher.stopID)
	require.Zero(t, handler.capacity.usedRAMMB)
	require.Zero(t, handler.capacity.usedSharedVCPU)
	require.Zero(t, handler.capacity.reservedRAMMB)
	require.Zero(t, handler.capacity.reservedSharedVCPU)
}

func requireLaunchReply(t *testing.T, msg *nats.Msg) *pb.MicroVMLaunchResponse {
	t.Helper()

	var reply pb.MicroVMLaunchResponse
	require.NoError(t, nats.UnmarshalProto(msg, &reply))

	return &reply
}

func launchMsg(t *testing.T, reply string, req *pb.MicroVMLaunchRequest) *nats.Msg {
	t.Helper()

	data, err := proto.Marshal(req)
	require.NoError(t, err)

	return &nats.Msg{
		Reply: reply,
		Data:  data,
	}
}
