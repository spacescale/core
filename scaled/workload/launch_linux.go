//go:build linux

// Package workload handles targeted microVM launch commands after placement wins.
//
// The package validates launch messages, commits reserved capacity, invokes the
// local microvm launcher, and publishes accepted only after guestd hello. On boot
// or reply failure it reverts capacity and tears down any started VM.
package workload

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/spacescale/core/scaled/workload/microvm"
	"github.com/spacescale/core/shared/nats"
	"github.com/spacescale/core/shared/pb/v1"
)

const (
	// microVMLaunchBootTimeout is a failure guard, not the expected boot time.
	// The guestd path should be fast; if Firecracker cannot start and send hello
	// inside this window, the node should reject the launch and free capacity.
	microVMLaunchBootTimeout = 10 * time.Second
)

// launcher abstracts the microVM launch boundary used by the launch handler.
// The concrete microvm.Launcher owns Linux, Firecracker, and KVM-specific
// boot behavior that sits below workload orchestration.
type launcher interface {
	Launch(ctx context.Context, request microvm.LaunchRequest) (*microvm.ActiveVM, error)
	Stop(ctx context.Context, microvmID string) error
}

// imageConfigResolver names the host-side OCI lookup dependency used by launch.
//
// Keeping it as a field on launchHandler lets tests replace the registry lookup
// with a small stub without pulling registry behavior into every launch test.
type imageConfigResolver func(context.Context, string) (resolvedOCIConfig, error)

// launchHandler handles targeted microVM launch commands after placement wins.
type launchHandler struct {
	logger             *slog.Logger
	capacity           *Capacity
	bootID             string
	launcher           launcher
	resolveImageConfig imageConfigResolver
}

// newLaunchHandler constructs a launch handler for one node boot identity.
func newLaunchHandler(logger *slog.Logger, capacity *Capacity, bootID string, launcher launcher) *launchHandler {
	return &launchHandler{
		logger:             logger,
		capacity:           capacity,
		bootID:             bootID,
		launcher:           launcher,
		resolveImageConfig: resolveOCIConfig,
	}
}

// register connects the launch handler to the node's specific targeted inbox.
// This subject includes the boot ID to guarantee that stale launch commands
// from previous boot lifecycles are naturally dropped.
func (h *launchHandler) register(ctx context.Context, client *nats.Client) (string, error) {
	subject := nats.NodeMicroVMLaunchSubject(h.bootID)
	_, err := client.Subscribe(subject, func(msg *nats.Msg) error {
		return h.handle(ctx, client, msg)
	})
	if err != nil {
		return "", err
	}
	return subject, nil
}

// handle validates a targeted launch command, resolves OCI runtime metadata,
// boots the local microVM, and only then acknowledges the request.
//
// The order matters: once capacity is committed, any failure in OCI lookup,
// launch-request resolution, VM boot, or reply publication must revert that
// capacity before returning.
func (h *launchHandler) handle(ctx context.Context, client *nats.Client, msg *nats.Msg) error {
	if msg.Reply == "" {
		return errors.New("microvm launch request missing reply subject")
	}

	var req pb.MicroVMLaunchRequest
	if err := nats.UnmarshalProto(msg, &req); err != nil {
		return err
	}
	if req.GetMicrovmId() == "" {
		return errors.New("microvm launch request missing microvm id")
	}

	if _, err := SpecFromShape(req.GetShape()); err != nil {
		return err
	}

	committedSpec, ok := h.capacity.Commit(req.GetMicrovmId())
	if !ok {
		return client.PublishProto(msg.Reply, &pb.MicroVMLaunchResponse{
			Accepted:     false,
			ErrorMessage: "reservation expired or not found",
		})
	}

	h.logger.Info("won microvm placement auction",
		"microvm_id", req.GetMicrovmId(),
		"vcpu", committedSpec.VCPU,
		"ram_mb", committedSpec.RAM,
		"cpu_mode", CPUModeLogValue(req.GetShape()),
		"volume_mb", req.GetShape().GetVolumeMb(),
	)

	launchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), microVMLaunchBootTimeout)
	defer cancel()

	cfg, err := h.resolveImageConfig(launchCtx, req.GetImageRef())
	if err != nil {
		return h.rejectLaunch(client, msg.Reply, committedSpec, err)
	}

	launchReq, err := resolveLaunchRequest(
		req.GetMicrovmId(),
		committedSpec,
		req.GetImageRef(),
		req.GetEnv(),
		req.GetRuntimePort(),
		cfg,
	)
	if err != nil {
		return h.rejectLaunch(client, msg.Reply, committedSpec, err)
	}

	active, err := h.launcher.Launch(launchCtx, launchReq)
	if err != nil {
		return h.rejectLaunch(client, msg.Reply, committedSpec, err)
	}

	reply := &pb.MicroVMLaunchResponse{
		Accepted: true,
	}

	if err := client.PublishProto(msg.Reply, reply); err != nil {
		if active != nil {
			err = errors.Join(err, h.launcher.Stop(context.WithoutCancel(ctx), active.MicroVMID))
		}
		h.capacity.Revert(committedSpec)
		return err
	}

	return nil
}

func (h *launchHandler) rejectLaunch(client *nats.Client, replySubject string, committedSpec HardwareSpec, err error) error {
	h.capacity.Revert(committedSpec)
	publishErr := client.PublishProto(replySubject, &pb.MicroVMLaunchResponse{
		Accepted:     false,
		ErrorMessage: err.Error(),
	})

	return errors.Join(err, publishErr)
}
