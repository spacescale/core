//go:build linux

// Executor handles targeted microVM launch commands after placement wins.
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

type Executor struct {
	logger   *slog.Logger
	capacity *Capacity
	bootID   string
	launcher *microvm.Launcher
}

func NewExecutor(logger *slog.Logger, capacity *Capacity, bootID string, launcher *microvm.Launcher) *Executor {
	return &Executor{
		logger:   logger,
		capacity: capacity,
		bootID:   bootID,
		launcher: launcher,
	}
}

// Register connects the executor to the node's specific targeted inbox.
// This subject includes the boot ID to guarantee that stale launch commands
// from previous boot lifecycles are naturally dropped.
func (e *Executor) Register(ctx context.Context, client *nats.Client) (string, error) {
	subject := nats.NodeMicroVMLaunchSubject(e.bootID)
	_, err := client.Subscribe(subject, func(msg *nats.Msg) error {
		return e.handle(ctx, client, msg)
	})
	if err != nil {
		return "", err
	}
	return subject, nil
}

func (e *Executor) handle(ctx context.Context, client *nats.Client, msg *nats.Msg) error {
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

	committedSpec, ok := e.capacity.Commit(req.GetMicrovmId())
	if !ok {
		return client.PublishProto(msg.Reply, &pb.MicroVMLaunchResponse{
			Accepted:     false,
			ErrorMessage: "reservation expired or not found",
		})
	}

	e.logger.Info("won microvm placement auction",
		"microvm_id", req.GetMicrovmId(),
		"vcpu", committedSpec.VCPU,
		"ram_mb", committedSpec.RAM,
		"cpu_mode", CPUModeLogValue(req.GetShape()),
		"volume_mb", req.GetShape().GetVolumeMb(),
	)

	launchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), microVMLaunchBootTimeout)
	defer cancel()

	active, err := e.launcher.Launch(launchCtx, microvm.LaunchRequest{
		MicroVMID: req.GetMicrovmId(),
		VCPU:      committedSpec.VCPU,
		RAMMB:     committedSpec.RAM,
	})
	if err != nil {
		e.capacity.Revert(committedSpec)
		publishErr := client.PublishProto(msg.Reply, &pb.MicroVMLaunchResponse{
			Accepted:     false,
			ErrorMessage: err.Error(),
		})
		return errors.Join(err, publishErr)
	}

	reply := &pb.MicroVMLaunchResponse{
		Accepted: true,
	}

	if err := client.PublishProto(msg.Reply, reply); err != nil {
		if active != nil {
			err = errors.Join(err, e.launcher.Stop(context.WithoutCancel(ctx), active.MicroVMID))
		}
		e.capacity.Revert(committedSpec)
		return err
	}

	return nil
}
