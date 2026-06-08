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

type publishedReply struct {
	subject string
	msg     proto.Message
}

type fakePublisher struct {
	published []publishedReply
	err       error
}

func (f *fakePublisher) PublishProto(subject string, message proto.Message) error {
	f.published = append(f.published, publishedReply{
		subject: subject,
		msg:     message,
	})
	return f.err
}

func newTestLaunchHandler(t *testing.T, launcher launcher) *launchHandler {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	capacity := NewCapacity(65536, 8)
	return newLaunchHandler(log, capacity, "boot-123", launcher)
}

func TestLaunchHandlerHandleRejectedMissingReplySubject(t *testing.T) {
	launcher := &fakeLauncher{}
	handler := newTestLaunchHandler(t, launcher)
	publisher := &fakePublisher{}

	msg := launchMsg(t, "", &pb.MicroVMLaunchRequest{
		MicrovmId: "vm-1",
		Shape: &pb.MicroVMShape{
			Vcpu:    2,
			RamMb:   2048,
			CpuMode: pb.CpuMode_CPU_MODE_SHARED,
		},
	})
	err := handler.handle(context.Background(), publisher, msg)
	require.ErrorContains(t, err, "missing reply subject")
	require.False(t, launcher.launchCalled)
	require.Empty(t, publisher.published)
}

func TestLaunchHandlerHandleRejectsInvalidProto(t *testing.T) {
	launcher := &fakeLauncher{}
	handler := newTestLaunchHandler(t, launcher)
	publisher := &fakePublisher{}

	msg := &nats.Msg{
		Reply: "reply.subject",
		Data:  []byte("not-proto"),
	}

	err := handler.handle(context.Background(), publisher, msg)
	require.ErrorContains(t, err, "unmarshal proto")
	require.False(t, launcher.launchCalled)
	require.Empty(t, publisher.published)
}

func TestLaunchHandlerHandleRejectsMissingMicroVMID(t *testing.T) {
	launcher := &fakeLauncher{}
	handler := newTestLaunchHandler(t, launcher)
	publisher := &fakePublisher{}

	msg := launchMsg(t, "reply.subject", &pb.MicroVMLaunchRequest{
		Shape: &pb.MicroVMShape{
			Vcpu:    2,
			RamMb:   2048,
			CpuMode: pb.CpuMode_CPU_MODE_SHARED,
		},
	})

	err := handler.handle(context.Background(), publisher, msg)
	require.ErrorContains(t, err, "missing microvm id")
	require.False(t, launcher.launchCalled)
	require.Empty(t, publisher.published)
}

func TestLaunchHandlerHandleRejectsInvalidShape(t *testing.T) {
	launcher := &fakeLauncher{}
	handler := newTestLaunchHandler(t, launcher)
	publisher := &fakePublisher{}

	msg := launchMsg(t, "reply.subject", &pb.MicroVMLaunchRequest{
		MicrovmId: "vm-1",
		Shape: &pb.MicroVMShape{
			Vcpu:  2,
			RamMb: 2048,
		},
	})

	err := handler.handle(context.Background(), publisher, msg)
	require.ErrorIs(t, err, ErrInvalidMicroVMShape)
	require.False(t, launcher.launchCalled)
	require.Empty(t, publisher.published)
}

func TestLaunchHandlerHandleRejectsMissingReservation(t *testing.T) {
	launcher := &fakeLauncher{}
	handler := newTestLaunchHandler(t, launcher)
	publisher := &fakePublisher{}

	msg := launchMsg(t, "reply.subject", &pb.MicroVMLaunchRequest{
		MicrovmId: "vm-1",
		Shape: &pb.MicroVMShape{
			Vcpu:    2,
			RamMb:   2048,
			CpuMode: pb.CpuMode_CPU_MODE_SHARED,
		},
	})

	err := handler.handle(context.Background(), publisher, msg)
	require.NoError(t, err)
	require.False(t, launcher.launchCalled)
	require.Len(t, publisher.published, 1)
	require.Equal(t, "reply.subject", publisher.published[0].subject)

	reply, ok := publisher.published[0].msg.(*pb.MicroVMLaunchResponse)
	require.True(t, ok)
	require.False(t, reply.GetAccepted())
	require.Equal(t, "reservation expired or not found", reply.GetErrorMessage())
}

func TestLaunchHandlerHandleRevertsCapacityWhenLaunchFails(t *testing.T) {
	launchErr := errors.New("launch failed")
	launcher := &fakeLauncher{launchErr: launchErr}
	handler := newTestLaunchHandler(t, launcher)
	publisher := &fakePublisher{}

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

	err := handler.handle(context.Background(), publisher, msg)
	require.ErrorIs(t, err, launchErr)

	require.True(t, launcher.launchCalled)
	require.Equal(t, "vm-1", launcher.launchReq.MicroVMID)
	require.Equal(t, uint32(2), launcher.launchReq.VCPU)
	require.Equal(t, uint64(2048), launcher.launchReq.RAMMB)
	require.False(t, launcher.stopCalled)

	require.Len(t, publisher.published, 1)
	reply := requireLaunchReply(t, publisher.published[0].msg)
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
	publisher := &fakePublisher{}

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

	err := handler.handle(context.Background(), publisher, msg)
	require.NoError(t, err)

	require.True(t, launcher.launchCalled)
	require.False(t, launcher.stopCalled)

	require.Len(t, publisher.published, 1)
	reply := requireLaunchReply(t, publisher.published[0].msg)
	require.True(t, reply.GetAccepted())
	require.Empty(t, reply.GetErrorMessage())

	require.Equal(t, uint64(2048), handler.capacity.usedRAMMB)
	require.Equal(t, uint32(2), handler.capacity.usedSharedVCPU)
	require.Zero(t, handler.capacity.reservedRAMMB)
	require.Zero(t, handler.capacity.reservedSharedVCPU)
}

func TestLaunchHandlerHandleStopsVMAndRevertsCapacityWhenPublishFails(t *testing.T) {
	publishErr := errors.New("publish failed")
	launcher := &fakeLauncher{
		launchVM: &microvm.ActiveVM{MicroVMID: "vm-1"},
	}
	handler := newTestLaunchHandler(t, launcher)
	publisher := &fakePublisher{err: publishErr}

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

	err := handler.handle(context.Background(), publisher, msg)
	require.ErrorIs(t, err, publishErr)

	require.True(t, launcher.launchCalled)
	require.True(t, launcher.stopCalled)
	require.Equal(t, "vm-1", launcher.stopID)

	require.Len(t, publisher.published, 1)
	require.Zero(t, handler.capacity.usedRAMMB)
	require.Zero(t, handler.capacity.usedSharedVCPU)
	require.Zero(t, handler.capacity.reservedRAMMB)
	require.Zero(t, handler.capacity.reservedSharedVCPU)
}

func requireLaunchReply(t *testing.T, msg proto.Message) *pb.MicroVMLaunchResponse {
	t.Helper()

	reply, ok := msg.(*pb.MicroVMLaunchResponse)
	require.True(t, ok)

	return reply
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
