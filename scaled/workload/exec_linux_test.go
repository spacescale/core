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

type fakeVMM struct {
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

func (f *fakeVMM) Launch(ctx context.Context, req microvm.LaunchRequest) (*microvm.ActiveVM, error) {
	f.launchCalled = true
	f.launchCtx = ctx
	f.launchReq = req
	return f.launchVM, f.launchErr
}

func (f *fakeVMM) Stop(ctx context.Context, microvmID string) error {
	f.stopCalled = true
	f.stopCtx = ctx
	f.stopID = microvmID
	return f.stopErr
}

func newTestExecutor(t *testing.T, vmm vmm) *executor {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	capacity := NewCapacity(65536, 16)
	exec := newExecutor(log, capacity, "boot-123", vmm)
	exec.resolveImageConfig = func(context.Context, string) (resolvedOCIConfig, error) {
		return resolvedOCIConfig{
			ImageDigest: "sha256:test",
			Entrypoint:  []string{"app"},
			Cmd:         []string{"serve"},
		}, nil
	}
	exec.materializeWorkload = func(context.Context, artifactCacheScope, string) (materializedArtifact, error) {
		return materializedArtifact{
			ImageDigest:  "sha256:test",
			ArtifactPath: "/var/lib/spacescale/workloads/workspaces/ws-1/artifacts/sha256/test/artifact.erofs",
		}, nil
	}

	return exec
}

func TestExecutorHandleRejectsMissingReplySubject(t *testing.T) {
	v := &fakeVMM{}
	exec := newTestExecutor(t, v)

	msg := launchMsg(t, "", &pb.MicroVMLaunchRequest{
		MicrovmId: "vm-1",
		Shape: &pb.MicroVMShape{
			Vcpu:    2,
			RamMb:   2048,
			CpuMode: pb.CpuMode_CPU_MODE_SHARED,
		},
	})

	err := exec.handle(t.Context(), newTestClient(t, startTestNATSServer(t)), msg)
	require.ErrorContains(t, err, "missing reply subject")
	require.False(t, v.launchCalled)
}

func TestExecutorHandleRejectsInvalidProto(t *testing.T) {
	v := &fakeVMM{}
	exec := newTestExecutor(t, v)

	msg := &nats.Msg{
		Reply: "reply.subject",
		Data:  []byte("not-proto"),
	}

	err := exec.handle(t.Context(), newTestClient(t, startTestNATSServer(t)), msg)
	require.ErrorContains(t, err, "unmarshal proto")
	require.False(t, v.launchCalled)
}

func TestExecutorHandleRejectsMissingMicroVMID(t *testing.T) {
	v := &fakeVMM{}
	exec := newTestExecutor(t, v)

	msg := launchMsg(t, "reply.subject", &pb.MicroVMLaunchRequest{
		Shape: &pb.MicroVMShape{
			Vcpu:    2,
			RamMb:   2048,
			CpuMode: pb.CpuMode_CPU_MODE_SHARED,
		},
	})

	err := exec.handle(t.Context(), newTestClient(t, startTestNATSServer(t)), msg)
	require.ErrorContains(t, err, "missing microvm id")
	require.False(t, v.launchCalled)
}

func TestExecutorHandleRejectsInvalidShape(t *testing.T) {
	v := &fakeVMM{}
	exec := newTestExecutor(t, v)

	msg := launchMsg(t, "reply.subject", &pb.MicroVMLaunchRequest{
		MicrovmId: "vm-1",
		Shape: &pb.MicroVMShape{
			Vcpu:  2,
			RamMb: 2048,
		},
	})

	err := exec.handle(t.Context(), newTestClient(t, startTestNATSServer(t)), msg)
	require.ErrorIs(t, err, ErrInvalidMicroVMShape)
	require.False(t, v.launchCalled)
}

func TestExecutorHandleRejectsMissingReservation(t *testing.T) {
	v := &fakeVMM{}
	exec := newTestExecutor(t, v)
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

	err := exec.handle(t.Context(), client, msg)
	require.NoError(t, err)
	require.False(t, v.launchCalled)

	reply := requireLaunchReply(t, receivePublishedMsg(t, replies))
	require.False(t, reply.GetAccepted())
	require.Equal(t, "reservation expired or not found", reply.GetErrorMessage())
}

func TestExecutorHandleRejectsOCIConfigResolutionFailure(t *testing.T) {
	resolverErr := errors.New("resolve failed")
	v := &fakeVMM{}
	exec := newTestExecutor(t, v)
	exec.resolveImageConfig = func(context.Context, string) (resolvedOCIConfig, error) {
		return resolvedOCIConfig{}, resolverErr
	}
	client := newTestClient(t, startTestNATSServer(t))
	replies := capturePublishedMsg(t, client)

	_, ok := exec.capacity.Reserve("vm-1", HardwareSpec{
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

	err := exec.handle(t.Context(), client, msg)
	require.ErrorIs(t, err, resolverErr)
	require.False(t, v.launchCalled)

	reply := requireLaunchReply(t, receivePublishedMsg(t, replies))
	require.False(t, reply.GetAccepted())
	require.Equal(t, "resolve failed", reply.GetErrorMessage())
}

func TestExecutorHandleRevertsCapacityWhenLaunchFails(t *testing.T) {
	launchErr := errors.New("launch failed")
	v := &fakeVMM{launchErr: launchErr}
	exec := newTestExecutor(t, v)
	client := newTestClient(t, startTestNATSServer(t))
	replies := capturePublishedMsg(t, client)

	_, ok := exec.capacity.Reserve("vm-1", HardwareSpec{
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

	err := exec.handle(t.Context(), client, msg)
	require.ErrorIs(t, err, launchErr)

	require.True(t, v.launchCalled)
	require.Equal(t, "vm-1", v.launchReq.MicroVMID)
	require.Equal(t, uint32(2), v.launchReq.VCPU)
	require.Equal(t, uint64(2048), v.launchReq.RAMMB)
	require.Equal(t, "ghcr.io/acme/app:latest", v.launchReq.ImageRef)
	require.Equal(t, "sha256:test", v.launchReq.ImageDigest)
	require.Equal(t, []string{"app", "serve"}, v.launchReq.Command)
	require.Equal(t, map[string]string{"FOO": "bar"}, v.launchReq.Env)
	require.Equal(t, uint32(9090), v.launchReq.RuntimePort)
	require.False(t, v.stopCalled)

	reply := requireLaunchReply(t, receivePublishedMsg(t, replies))
	require.False(t, reply.GetAccepted())
	require.Equal(t, "launch failed", reply.GetErrorMessage())

	require.Zero(t, exec.capacity.usedRAMMB)
	require.Zero(t, exec.capacity.usedSharedVCPU)
	require.Zero(t, exec.capacity.reservedRAMMB)
	require.Zero(t, exec.capacity.reservedSharedVCPU)
}

func TestExecutorHandlePublishesAcceptedAfterLaunch(t *testing.T) {
	v := &fakeVMM{
		launchVM: &microvm.ActiveVM{MicroVMID: "vm-1"},
	}
	exec := newTestExecutor(t, v)
	client := newTestClient(t, startTestNATSServer(t))
	replies := capturePublishedMsg(t, client)

	_, ok := exec.capacity.Reserve("vm-1", HardwareSpec{
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

	err := exec.handle(t.Context(), client, msg)
	require.NoError(t, err)

	require.True(t, v.launchCalled)
	require.False(t, v.stopCalled)

	reply := requireLaunchReply(t, receivePublishedMsg(t, replies))
	require.True(t, reply.GetAccepted())
	require.Empty(t, reply.GetErrorMessage())

	require.Equal(t, uint64(2048), exec.capacity.usedRAMMB)
	require.Equal(t, uint32(2), exec.capacity.usedSharedVCPU)
	require.Zero(t, exec.capacity.reservedRAMMB)
	require.Zero(t, exec.capacity.reservedSharedVCPU)
}

func TestExecutorHandleStopsVMAndRevertsCapacityWhenPublishFails(t *testing.T) {
	v := &fakeVMM{
		launchVM: &microvm.ActiveVM{MicroVMID: "vm-1"},
	}
	exec := newTestExecutor(t, v)
	client := newTestClient(t, startTestNATSServer(t))
	client.Close()

	_, ok := exec.capacity.Reserve("vm-1", HardwareSpec{
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

	err := exec.handle(t.Context(), client, msg)
	require.Error(t, err)

	require.True(t, v.launchCalled)
	require.True(t, v.stopCalled)
	require.Equal(t, "vm-1", v.stopID)
	require.Zero(t, exec.capacity.usedRAMMB)
	require.Zero(t, exec.capacity.usedSharedVCPU)
	require.Zero(t, exec.capacity.reservedRAMMB)
	require.Zero(t, exec.capacity.reservedSharedVCPU)
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
