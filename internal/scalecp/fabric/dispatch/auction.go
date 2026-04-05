package dispatch

import (
	"cmp"
	"errors"
	"slices"

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

	msgs, err := d.nats.GatherProto(nats.NodeAuctionSubject(req.Region), &pb.AuctionRequest{MicrovmId: req.MicroVMID.String(), Shape: req.Shape})
	if err != nil {
		return Winner{}, err
	}

	bids := make([]*pb.AuctionReply, 0, len(msgs))
	for _, msg := range msgs {
		reply := &pb.AuctionReply{}
		if err := nats.UnmarshalProto(msg, reply); err != nil {
			continue
		}
		if reply.MicrovmId != req.MicroVMID.String() {
			continue
		}
		if reply.NodeId == "" || reply.BootId == "" {
			continue
		}
		bids = append(bids, reply)
	}
	if len(bids) == 0 {
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

	// Assuming 'bids' is [] *pb.AuctionReply
	slices.SortFunc(bids, func(a, b *pb.AuctionReply) int {
		// Step 1: Sort by Free RAM in DESCENDING order.
		if r := cmp.Compare(b.FreeRamMb, a.FreeRamMb); r != 0 {
			return r
		}

		// Step 2: Tie-breaker - Node ID in ASCENDING order.
		if r := cmp.Compare(a.NodeId, b.NodeId); r != 0 {
			return r
		}

		// Step 3: Final tie-breaker - Boot ID in ASCENDING order.
		return cmp.Compare(a.BootId, b.BootId)
	})
	winner := bids[0]
	selectedArgs := []any{
		"app_id", req.AppID,
		"deployment_id", req.DeploymentID,
		"microvm_id", req.MicroVMID,
		"region", req.Region,
		"node_id", winner.NodeId,
		"boot_id", winner.BootId,
		"free_ram_mb", winner.FreeRamMb,
		"bid_count", len(bids),
	}
	selectedArgs = append(selectedArgs, shapeLogAttrs(req.Shape)...)
	d.logger.Info("placement auction selected winner", selectedArgs...)

	return Winner{NodeID: winner.NodeId, BootID: winner.BootId, FreeRamMB: winner.FreeRamMb}, nil
}
