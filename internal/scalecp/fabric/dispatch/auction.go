package dispatch

import (
	"errors"
	"fmt"

	"github.com/spacescale/core/internal/shared/nats"
	"github.com/spacescale/core/internal/shared/pb/v1"
)

var (
	ErrNoAuctionBids = errors.New("no auction bids")
)

func (d *Dispatcher) auction(req Request) (Winner, error) {
	logArgs := []any{
		"app_id", req.AppID,
		"deployment_id", req.DeploymentID,
		"microvm_id", req.MicroVMID,
		"region", req.Region,
	}
	logArgs = append(logArgs, shapeLogAttrs(req.Shape)...)
	d.logger.Info("starting placement auction", logArgs...)

	// Placement is intentionally first-response-wins for now. The NATS client arms
	// the private inbox with AutoUnsubscribe(1) before publishing the auction, so the
	// server drops slower bids instead of forwarding them to this control plane.
	msg, err := d.nats.FirstReplyProto(nats.NodeAuctionSubject(req.Region), &pb.AuctionRequest{MicrovmId: req.MicroVMID.String(), Shape: req.Shape})
	if err != nil {
		return Winner{}, err
	}
	if msg == nil {
		warnArgs := []any{
			"app_id", req.AppID,
			"deployment_id", req.DeploymentID,
			"microvm_id", req.MicroVMID,
			"region", req.Region,
		}
		warnArgs = append(warnArgs, shapeLogAttrs(req.Shape)...)
		d.logger.Warn("placement auction received no bids", warnArgs...)
		return Winner{}, ErrNoAuctionBids
	}

	reply := &pb.AuctionReply{}
	if err := nats.UnmarshalProto(msg, reply); err != nil {
		return Winner{}, fmt.Errorf("decode first auction reply: %w", err)
	}
	if reply.MicrovmId != req.MicroVMID.String() {
		return Winner{}, errors.New("auction reply microvm id mismatch")
	}
	if reply.NodeId == "" || reply.BootId == "" {
		return Winner{}, errors.New("auction reply missing node identity")
	}

	selectedArgs := []any{
		"app_id", req.AppID,
		"deployment_id", req.DeploymentID,
		"microvm_id", req.MicroVMID,
		"region", req.Region,
		"node_id", reply.NodeId,
		"boot_id", reply.BootId,
		"free_ram_mb", reply.FreeRamMb,
	}
	selectedArgs = append(selectedArgs, shapeLogAttrs(req.Shape)...)
	d.logger.Info("placement auction selected first responder", selectedArgs...)

	return Winner{NodeID: reply.NodeId, BootID: reply.BootId}, nil
}
