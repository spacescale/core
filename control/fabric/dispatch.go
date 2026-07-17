// Package fabric owns control's outbound NATS placement dispatch. It broadcasts
// workload requirements over the fabric before issuing targeted launch commands.
package fabric

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/spacescale/core/control/tenant"
	"github.com/spacescale/core/shared/nats"
	pb "github.com/spacescale/core/shared/pb/v1"
)

const (
	microVMLaunchTimeout   = 15 * time.Second
	launchLogAttrBaseCap   = 12
	warnLogExtraAttrCap    = 2
	noAuctionBidLogBaseCap = 8
)

var (
	// ErrLaunchRejected marks a node-side launch refusal after placement succeeded.
	ErrLaunchRejected = errors.New("microvm launch rejected")

	// ErrNoAuctionBids means no scaled node replied to a placement auction.
	ErrNoAuctionBids = errors.New("no auction bids")
)

// Dispatcher coordinates placement auctions and targeted launch commands.
type Dispatcher struct {
	workloads *tenant.WorkloadService
	nats      *nats.Client
	logger    *slog.Logger
}

// Request is the durable workload state needed to dispatch one microVM launch.
type Request struct {
	WorkloadID   uuid.UUID
	DeploymentID uuid.UUID
	MicroVMID    uuid.UUID
	WorkspaceID  string

	// Region is the currently preferred/selected region. For automatic
	// placement it starts as the first candidate and is updated when a later
	// regional auction wins.
	Region string
	// Regions is the ordered auction candidate list. Explicit requests contain
	// one region. Automatic requests may contain a geo-priority fallback chain.
	Regions []string

	Shape       *pb.MicroVMShape
	ImageRef    string
	Env         map[string]string
	RuntimePort uint32
}

// Winner is the node identity selected by a placement auction.
type Winner struct {
	NodeID string
	BootID string
	Region string
}

// NewDispatcher creates a dispatcher bound to tenant lifecycle services and the
// NATS communication fabric.
func NewDispatcher(workloads *tenant.WorkloadService, client *nats.Client, logger *slog.Logger) *Dispatcher {
	return &Dispatcher{
		workloads: workloads,
		nats:      client,
		logger:    logger.With("component", "fabric"),
	}
}

// Launch auctions a microVM placement, sends the launch command, and records dispatch state.
func (d *Dispatcher) Launch(ctx context.Context, req Request) error {
	startedAt := time.Now()
	winner, err := d.place(req)
	if err != nil {
		return returnLaunchError(err, d.markFailed(ctx, req, err.Error()))
	}
	req.Region = winner.Region

	shapeAttrs := shapeLogAttrs(req.Shape)
	launchArgs := make([]any, 0, launchLogAttrBaseCap+len(shapeAttrs))
	launchArgs = append(launchArgs,
		"workload_id", req.WorkloadID,
		"microvm_id", req.MicroVMID,
		"region", req.Region,
		"node_id", winner.NodeID,
	)
	launchArgs = append(launchArgs, shapeAttrs...)
	nodeID, err := uuid.Parse(winner.NodeID)
	if err != nil {
		return err
	}
	if err := d.workloads.MarkDeploying(ctx, tenant.DispatchAssignment{
		WorkloadID:   req.WorkloadID,
		DeploymentID: req.DeploymentID,
		MicroVMID:    req.MicroVMID,
		NodeID:       nodeID,
	}); err != nil {
		return err
	}

	resp := new(pb.MicroVMLaunchResponse)
	if err := d.nats.RequestProto(
		nats.NodeMicroVMLaunchSubject(winner.BootID),
		&pb.MicroVMLaunchRequest{
			MicrovmId:   req.MicroVMID.String(),
			WorkspaceId: req.WorkspaceID,
			Shape:       req.Shape,
			ImageRef:    req.ImageRef,
			Env:         req.Env,
			RuntimePort: req.RuntimePort,
		},
		resp,
		microVMLaunchTimeout,
	); err != nil {
		warnArgs := make([]any, 0, len(launchArgs)+warnLogExtraAttrCap)
		warnArgs = append(warnArgs, launchArgs...)
		warnArgs = append(warnArgs, "error", err)
		d.logger.Warn("microvm launch command failed", warnArgs...)

		return returnLaunchError(err, d.markFailed(ctx, req, err.Error()))
	}

	if !resp.GetAccepted() {
		reason := resp.GetErrorMessage()
		if reason == "" {
			reason = "launch rejected"
		}
		if err := d.markFailed(ctx, req, reason); err != nil {
			return err
		}
		warnArgs := make([]any, 0, len(launchArgs)+warnLogExtraAttrCap)
		warnArgs = append(warnArgs, launchArgs...)
		warnArgs = append(warnArgs, "reason", reason)
		d.logger.Warn("microvm launch rejected", warnArgs...)

		return fmt.Errorf("%w: %s", ErrLaunchRejected, reason)
	}

	acceptedArgs := append([]any{}, launchArgs...)
	acceptedArgs = append(acceptedArgs,
		"duration_ms", time.Since(startedAt).Milliseconds(),
	)
	d.logger.Info("microvm launch accepted", acceptedArgs...)

	if err := d.workloads.MarkMicroVMStarting(ctx, req.MicroVMID); err != nil {
		d.logger.Error("failed to mark microvm starting",
			"workload_id", req.WorkloadID,
			"microvm_id", req.MicroVMID,
			"node_id", winner.NodeID,
			"error", err,
		)
	}

	return nil
}

// place tries regional auctions in candidate order. Explicit placement has one
// candidate. Automatic placement falls back only when a region returns no bids.
func (d *Dispatcher) place(req Request) (Winner, error) {
	candidates := req.Regions
	if len(candidates) == 0 && req.Region != "" {
		candidates = []string{req.Region}
	}
	if len(candidates) == 0 {
		return Winner{}, ErrNoAuctionBids
	}

	var lastErr error
	for _, region := range candidates {
		regionReq := req
		regionReq.Region = region
		winner, err := auctionFn(d, regionReq)
		if err == nil {
			if region != req.Region && req.Region != "" {
				d.logger.Info("automatic placement fell back to region",
					"workload_id", req.WorkloadID,
					"microvm_id", req.MicroVMID,
					"requested_region", req.Region,
					"selected_region", region,
				)
			}
			return winner, nil
		}
		if !errors.Is(err, ErrNoAuctionBids) {
			return Winner{}, err
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = ErrNoAuctionBids
	}
	return Winner{}, lastErr
}

// auctionFn is the regional auction implementation. Tests may replace it.
var auctionFn = (*Dispatcher).auction

func (d *Dispatcher) auction(req Request) (Winner, error) {
	// Placement is intentionally first-response-wins for now. The NATS client arms
	// the private inbox with AutoUnsubscribe(1) before publishing the auction, so the
	// server drops slower bids instead of forwarding them to this control plane.
	msg, err := d.nats.FirstReplyProto(nats.NodeAuctionSubject(req.Region), &pb.AuctionRequest{MicrovmId: req.MicroVMID.String(), Shape: req.Shape})
	if err != nil {
		if errors.Is(err, nats.ErrNoReply) {
			d.logNoAuctionBids(req)

			return Winner{}, ErrNoAuctionBids
		}

		return Winner{}, err
	}
	if msg == nil {
		d.logNoAuctionBids(req)

		return Winner{}, ErrNoAuctionBids
	}

	reply := new(pb.AuctionReply)
	if err := nats.UnmarshalProto(msg, reply); err != nil {
		return Winner{}, fmt.Errorf("decode first auction reply: %w", err)
	}
	if reply.GetNodeId() == "" || reply.GetBootId() == "" {
		return Winner{}, errors.New("auction reply missing node identity")
	}

	return Winner{NodeID: reply.GetNodeId(), BootID: reply.GetBootId(), Region: req.Region}, nil
}

func (d *Dispatcher) logNoAuctionBids(req Request) {
	shapeAttrs := shapeLogAttrs(req.Shape)
	warnArgs := make([]any, 0, noAuctionBidLogBaseCap+len(shapeAttrs))
	warnArgs = append(warnArgs,
		"workload_id", req.WorkloadID,
		"microvm_id", req.MicroVMID,
		"region", req.Region,
	)
	warnArgs = append(warnArgs, shapeAttrs...)
	d.logger.Warn("placement auction received no bids", warnArgs...)
}

func returnLaunchError(launchErr, markErr error) error {
	if markErr == nil {
		return launchErr
	}

	return errors.Join(launchErr, markErr)
}

func (d *Dispatcher) markFailed(ctx context.Context, req Request, reason string) error {
	return d.workloads.MarkDispatchFailed(ctx, tenant.DispatchFailure{
		WorkloadID:   req.WorkloadID,
		DeploymentID: req.DeploymentID,
		MicroVMID:    req.MicroVMID,
		Reason:       reason,
	})
}

func shapeLogAttrs(shape *pb.MicroVMShape) []any {
	if shape == nil {
		return []any{"vcpu", uint32(0), "ram_mb", uint64(0), "cpu_mode", "unspecified", "volume_mb", uint64(0)}
	}

	cpuMode := "unspecified"
	if shape.GetCpuMode() == pb.CpuMode_CPU_MODE_SHARED {
		cpuMode = "shared"
	}
	if shape.GetCpuMode() == pb.CpuMode_CPU_MODE_PINNED {
		cpuMode = "pinned"
	}

	return []any{
		"vcpu", shape.GetVcpu(),
		"ram_mb", shape.GetRamMb(),
		"cpu_mode", cpuMode,
		"volume_mb", shape.GetVolumeMb(),
	}
}
