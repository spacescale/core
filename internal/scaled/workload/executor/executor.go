// Copyright (c) 2026 SpaceScale Systems Inc. All rights reserved.

//go:build linux

// Package executor handles targeted microVM launch commands after placement
// wins.
//
// The package validates launch messages, commits reserved capacity, invokes the
// local microvm launcher, and publishes accepted only after scoutd hello. On boot
// or reply failure it reverts capacity and tears down any started VM.
package executor

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/spacescale/core/internal/scaled/workload/microvm"
	"github.com/spacescale/core/internal/scaled/workload/placement"
	"github.com/spacescale/core/internal/shared/nats"
	pb "github.com/spacescale/core/internal/shared/pb/v1"
)

const (
	microVMLaunchAcceptedStatus = "starting"
	microVMLaunchFailedStatus   = "failed"

	// microVMLaunchBootTimeout is a failure guard, not the expected boot time.
	// The scoutd path should be fast; if Firecracker cannot start and send hello
	// inside this window, the node should reject the launch and free capacity.
	microVMLaunchBootTimeout = 10 * time.Second
)

type Executor struct {
	logger   *slog.Logger
	capacity *placement.Capacity
	bootID   string
	launcher *microvm.Launcher
}

func New(logger *slog.Logger, capacity *placement.Capacity, bootID string, launcher *microvm.Launcher) *Executor {
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
func (e *Executor) Register(client *nats.Client) error {
	subject := nats.NodeMicroVMLaunchSubject(e.bootID)
	_, err := client.Subscribe(subject, func(msg *nats.Msg) error {
		return e.handle(client, msg)
	})
	if err == nil {
		e.logger.Info("listening for direct launch commands", "subject", subject)
	}
	return err
}

func (e *Executor) handle(client *nats.Client, msg *nats.Msg) error {
	if msg.Reply == "" {
		return errors.New("microvm launch request missing reply subject")
	}

	var req pb.MicroVMLaunchRequest
	if err := nats.UnmarshalProto(msg, &req); err != nil {
		return err
	}
	if req.MicrovmId == "" {
		return errors.New("microvm launch request missing microvm id")
	}

	if _, err := placement.SpecFromShape(req.Shape); err != nil {
		return err
	}

	committedSpec, ok := e.capacity.Commit(req.MicrovmId)
	if !ok {
		return client.PublishProto(msg.Reply, &pb.MicroVMLaunchResponse{
			MicrovmId:    req.MicrovmId,
			Accepted:     false,
			Status:       microVMLaunchFailedStatus,
			ErrorMessage: "reservation expired or not found",
		})
	}

	e.logger.Info("won microvm placement auction",
		"microvm_id", req.MicrovmId,
		"vcpu", committedSpec.VCPU,
		"ram_mb", committedSpec.RAM,
		"cpu_mode", placement.CpuModeLogValue(req.GetShape()),
		"volume_mb", req.GetShape().GetVolumeMb(),
	)

	launchCtx, cancel := context.WithTimeout(context.Background(), microVMLaunchBootTimeout)
	defer cancel()

	active, err := e.launcher.Launch(launchCtx, microvm.LaunchRequest{
		MicroVMID: req.MicrovmId,
		VCPU:      committedSpec.VCPU,
		RAMMB:     committedSpec.RAM,
	})
	if err != nil {
		e.capacity.Revert(committedSpec)
		publishErr := client.PublishProto(msg.Reply, &pb.MicroVMLaunchResponse{
			MicrovmId:    req.MicrovmId,
			Accepted:     false,
			Status:       microVMLaunchFailedStatus,
			ErrorMessage: err.Error(),
		})
		return errors.Join(err, publishErr)
	}

	reply := &pb.MicroVMLaunchResponse{
		MicrovmId: req.MicrovmId,
		Accepted:  true,
		Status:    microVMLaunchAcceptedStatus,
	}

	if err := client.PublishProto(msg.Reply, reply); err != nil {
		if active != nil {
			err = errors.Join(err, e.launcher.Stop(context.Background(), active.MicroVMID))
		}
		e.capacity.Revert(committedSpec)
		return err
	}

	return nil
}
