package dispatch

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/spacescale/core/internal/scalecp/db/sqlc"
	"github.com/spacescale/core/internal/shared/nats"
	"github.com/spacescale/core/internal/shared/pb/v1"
)

const machineLaunchTimeout = 2 * time.Second

var ErrLaunchRejected = errors.New("machine launch rejected")

func (d *Dispatcher) Launch(ctx context.Context, req Request) error {
	winner, err := d.auction(req)
	if err != nil {
		return d.markFailed(ctx, req, err.Error())
	}
	d.logger.Info("dispatching launch command",
		"app_id", req.AppID,
		"deployment_id", req.DeploymentID,
		"machine_id", req.MachineID,
		"region", req.Region,
		"tier", req.Tier,
		"node_id", winner.NodeID,
		"boot_id", winner.BootID,
	)
	nodeID := winner.NodeID
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	qtx := d.queries.WithTx(tx)

	if _, err := qtx.AssignMachineToNode(ctx, sqlc.AssignMachineToNodeParams{
		ID:     req.MachineID,
		NodeID: &nodeID,
	}); err != nil {
		return err
	}

	if _, err := qtx.MarkDeploymentDeploying(ctx, req.DeploymentID); err != nil {
		return err
	}

	if _, err := qtx.MarkAppDeploying(ctx, req.AppID); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	resp := &pb.MachineLaunchResponse{}
	if err := d.nats.RequestProto(
		nats.NodeMachineLaunchSubject(winner.BootID),
		&pb.MachineLaunchRequest{
			MachineId: req.MachineID.String(),
			Tier:      req.Tier,
			ImageRef:  req.ImageRef,
			Env:       req.Env,
		},
		resp,
		machineLaunchTimeout,
	); err != nil {
		return d.markFailed(ctx, req, err.Error())
	}

	if !resp.Accepted {
		reason := resp.ErrorMessage
		if reason == "" {
			reason = "launch rejected"
		}
		if err := d.markFailed(ctx, req, reason); err != nil {
			return err
		}
		return fmt.Errorf("%w: %s", ErrLaunchRejected, reason)
	}

	d.logger.Info("launch command accepted",
		"app_id", req.AppID,
		"deployment_id", req.DeploymentID,
		"machine_id", req.MachineID,
		"region", req.Region,
		"tier", req.Tier,
		"node_id", winner.NodeID,
		"boot_id", winner.BootID,
		"status", resp.Status,
	)

	_, err = d.queries.MarkMachineStarting(ctx, req.MachineID)
	return err
}

func (d *Dispatcher) markFailed(ctx context.Context, req Request, reason string) error {
	errMsg := reason
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	qtx := d.queries.WithTx(tx)

	if _, err := qtx.MarkMachineFailed(ctx, sqlc.MarkMachineFailedParams{
		ID:           req.MachineID,
		ErrorMessage: &errMsg,
	}); err != nil {
		return err
	}

	if _, err := qtx.MarkDeploymentFailed(ctx, sqlc.MarkDeploymentFailedParams{
		ID:           req.DeploymentID,
		ErrorMessage: &errMsg,
	}); err != nil {
		return err
	}

	if _, err := qtx.MarkAppFailed(ctx, req.AppID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
