// Copyright (c) 2026 SpaceScale Systems Inc. All rights reserved.

package dispatch

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/spacescale/core/internal/scalecp/db/sqlc"
	"github.com/spacescale/core/internal/shared/nats"
	"github.com/spacescale/core/internal/shared/pb/v1"
)

const microVMLaunchTimeout = 15 * time.Second

var ErrLaunchRejected = errors.New("microvm launch rejected")

func (d *Dispatcher) Launch(ctx context.Context, req Request) error {
	winner, err := d.auction(req)
	if err != nil {
		return returnLaunchError(err, d.markFailed(ctx, req, err.Error()))
	}
	logArgs := []any{
		"app_id", req.AppID,
		"deployment_id", req.DeploymentID,
		"microvm_id", req.MicroVMID,
		"region", req.Region,
		"node_id", winner.NodeID,
		"boot_id", winner.BootID,
	}
	logArgs = append(logArgs, shapeLogAttrs(req.Shape)...)
	d.logger.Info("dispatching microvm launch command", logArgs...)
	nodeID, err := uuid.Parse(winner.NodeID)
	if err != nil {
		return err
	}
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	qtx := d.queries.WithTx(tx)

	if _, err := qtx.AssignMicroVMToNode(ctx, sqlc.AssignMicroVMToNodeParams{
		ID:     req.MicroVMID,
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

	resp := &pb.MicroVMLaunchResponse{}
	if err := d.nats.RequestProto(
		nats.NodeMicroVMLaunchSubject(winner.BootID),
		&pb.MicroVMLaunchRequest{
			MicrovmId:   req.MicroVMID.String(),
			Shape:       req.Shape,
			ImageRef:    req.ImageRef,
			Env:         req.Env,
			RuntimePort: req.RuntimePort,
		},
		resp,
		microVMLaunchTimeout,
	); err != nil {
		return returnLaunchError(err, d.markFailed(ctx, req, err.Error()))
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

	acceptedArgs := []any{
		"app_id", req.AppID,
		"deployment_id", req.DeploymentID,
		"microvm_id", req.MicroVMID,
		"region", req.Region,
		"node_id", winner.NodeID,
		"boot_id", winner.BootID,
		"status", resp.Status,
	}
	acceptedArgs = append(acceptedArgs, shapeLogAttrs(req.Shape)...)
	d.logger.Info("microvm launch command accepted", acceptedArgs...)

	if _, err := d.queries.MarkMicroVMStarting(ctx, req.MicroVMID); err != nil {
		d.logger.Error("failed to mark microvm starting",
			"app_id", req.AppID,
			"deployment_id", req.DeploymentID,
			"microvm_id", req.MicroVMID,
			"node_id", winner.NodeID,
			"boot_id", winner.BootID,
			"error", err,
		)
	}

	return nil
}

func returnLaunchError(launchErr, markErr error) error {
	if markErr == nil {
		return launchErr
	}
	return errors.Join(launchErr, markErr)
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

	if _, err := qtx.MarkMicroVMFailed(ctx, sqlc.MarkMicroVMFailedParams{
		ID:           req.MicroVMID,
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
