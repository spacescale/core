// Package placement implements the decentralized scheduling engine for the edge node.
//
// This file provides the Bidder, which is responsible for listening to regional
// control plane placement auctions over NATS. It validates the resolved microvm
// shape, consults the local Capacity ledger, and replies with a bid if the node
// can accommodate the workload.
package placement

import (
	"errors"
	"log/slog"
	"time"

	"github.com/spacescale/core/internal/shared/nats"
	pb "github.com/spacescale/core/internal/shared/pb/v1"
)

// reservationTTL defines the maximum time the node will hold physical resources
// in reserve while waiting for a Launch command before assuming it lost the auction.
const reservationTTL = 500 * time.Millisecond

// Bidder handles incoming placement auctions from the Control Plane.
// It acts as a strict network adapter, parsing NATS messages and invoking
// pure domain logic within the Capacity ledger.
type Bidder struct {
	logger   *slog.Logger
	capacity *Capacity
	nodeID   string
	bootID   string
	region   string
}

// NewBidder wires the NATS transport to the local capacity ledger.
func NewBidder(logger *slog.Logger, c *Capacity, nodeID, bootID, region string) *Bidder {
	return &Bidder{
		logger:   logger,
		capacity: c,
		nodeID:   nodeID,
		bootID:   bootID,
		region:   region,
	}
}

// Register connects the bidder to the NATS regional broadcast subject.
// This allows the orchestrator to explicitly control when the node begins
// accepting workloads.
func (b *Bidder) Register(client *nats.Client) error {
	subject := nats.NodeAuctionSubject(b.region)
	_, err := client.Subscribe(subject, func(msg *nats.Msg) error {
		return b.handle(client, msg)
	})
	if err == nil {
		b.logger.Info("listening for placement auctions", "subject", subject)
	}
	return err
}

func (b *Bidder) handle(client *nats.Client, msg *nats.Msg) error {
	if msg.Reply == "" {
		return errors.New("auction request missing reply subject")
	}

	var req pb.AuctionRequest
	if err := nats.UnmarshalProto(msg, &req); err != nil {
		return err
	}
	if req.MicrovmId == "" {
		return errors.New("auction request missing microvm id")
	}

	spec, err := SpecFromShape(req.Shape)
	if err != nil {
		return err
	}

	freeRAM, ok := b.capacity.Reserve(req.MicrovmId, spec, reservationTTL)
	if !ok {
		return nil
	}

	b.logger.Info(
		"submitted bid",
		"microvm_id", req.MicrovmId,
		"vcpu", req.GetShape().GetVcpu(),
		"ram_mb", req.GetShape().GetRamMb(),
		"cpu_mode", CpuModeLogValue(req.GetShape()),
		"free_ram_mb", freeRAM,
	)

	reply := &pb.AuctionReply{
		MicrovmId: req.MicrovmId,
		NodeId:    b.nodeID,
		BootId:    b.bootID,
		FreeRamMb: freeRAM,
	}

	if err := client.PublishProto(msg.Reply, reply); err != nil {
		b.capacity.Release(req.MicrovmId)
		return err
	}

	return nil
}
