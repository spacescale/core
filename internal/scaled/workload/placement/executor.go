// Package placement implements the decentralized scheduling engine for the edge node.
//
// This file provides the Executor, which is responsible for listening to targeted
// Control Plane launch commands over NATS. It verifies the intent, commits the
// previously held capacity reservation, and acknowledges the successful placement.
package placement

import (
	"errors"
	"log/slog"

	"github.com/spacescale/core/internal/shared/nats"
	pb "github.com/spacescale/core/internal/shared/pb/v1"
)

const (
	machineLaunchAcceptedStatus = "starting"
	machineLaunchFailedStatus   = "failed"
)

// Executor acts as the final decision boundary for the edge node during a placement auction.
// It listens to a targeted NATS inbox corresponding to the node's boot ID.
//
// When the Control Plane selects this node as the winner of an auction, it sends
// a MachineLaunchRequest here. The Executor is responsible for permanently committing
// the optimistically reserved capacity and initiating the workload boot sequence.
type Executor struct {
	logger   *slog.Logger
	capacity *Capacity
	bootID   string
}

// NewExecutor creates a new Executor wired to the local Capacity ledger.
func NewExecutor(logger *slog.Logger, c *Capacity, bootID string) *Executor {
	return &Executor{
		logger:   logger,
		capacity: c,
		bootID:   bootID,
	}
}

// Register connects the executor to the node's specific targeted inbox.
// This subject includes the boot ID to guarantee that stale launch commands
// from previous boot lifecycles are naturally dropped.
func (e *Executor) Register(client *nats.Client) error {
	subject := nats.NodeMachineLaunchSubject(e.bootID)
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
		return errors.New("machine launch request missing reply subject")
	}

	var req pb.MachineLaunchRequest
	if err := nats.UnmarshalProto(msg, &req); err != nil {
		return err
	}
	if req.MachineId == "" {
		return errors.New("machine launch request missing machine id")
	}

	if _, err := TranslateTier(req.Tier); err != nil {
		return err
	}

	committedSpec, ok := e.capacity.Commit(req.MachineId)
	if !ok {
		return client.PublishProto(msg.Reply, &pb.MachineLaunchResponse{
			MachineId:    req.MachineId,
			Accepted:     false,
			Status:       machineLaunchFailedStatus,
			ErrorMessage: "reservation expired or not found",
		})
	}

	pinnedStr := "shared"
	if committedSpec.IsPinned {
		pinnedStr = "pinned"
	}
	e.logger.Info("machine launch accepted",
		"machine_id", req.MachineId,
		"tier", req.Tier,
		"vcpu", committedSpec.VCPU,
		"ram_mb", committedSpec.RAM,
		"cpu_mode", pinnedStr,
	)

	reply := &pb.MachineLaunchResponse{
		MachineId: req.MachineId,
		Accepted:  true,
		Status:    machineLaunchAcceptedStatus,
	}

	if err := client.PublishProto(msg.Reply, reply); err != nil {
		e.capacity.Revert(committedSpec)
		return err
	}

	return nil
}
