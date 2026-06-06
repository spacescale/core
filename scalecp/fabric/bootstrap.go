package ingress

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/spacescale/core/internal/scalecp/service"
	"github.com/spacescale/core/internal/scalecp/service/fleet"
	"github.com/spacescale/core/internal/shared/nats"
	pb "github.com/spacescale/core/internal/shared/pb/v1"
)

var (
	maxInt32Uint32 = ^uint32(0) >> 1
	maxInt64Uint64 = ^uint64(0) >> 1
)

type BootstrapHandler struct {
	bootstrap *fleet.BootstrapService
	logger    *slog.Logger
}

func NewBootstrapHandler(services service.FleetServices, logger *slog.Logger) *BootstrapHandler {
	return &BootstrapHandler{bootstrap: services.Bootstrap, logger: logger}
}

func (h *BootstrapHandler) Register(ctx context.Context, client *nats.Client) error {
	if _, err := client.QueueSubscribe(nats.SubjectNodeBootstrap, nats.QueueNodeBootstrap, func(msg *nats.Msg) error {
		return h.handle(ctx, client, msg)
	}); err != nil {
		return err
	}

	return nil
}

func (h *BootstrapHandler) handle(ctx context.Context, client *nats.Client, msg *nats.Msg) error {
	reply := msg.Reply
	if reply == "" {
		return errors.New("bootstrap request is missing reply subject")
	}

	req := &pb.NodeBootstrapRequest{}
	if err := nats.UnmarshalProto(msg, req); err != nil {
		return h.reply(client, reply, &pb.NodeBootstrapResponse{Error: fleet.ErrInvalidBootstrapRequest.Error()})
	}

	input, err := validateBootstrapRequest(req)
	if err != nil {
		return h.reply(client, reply, &pb.NodeBootstrapResponse{Error: fleet.ErrInvalidBootstrapRequest.Error()})
	}

	out, err := h.bootstrap.Register(ctx, input)
	if err != nil {
		if !errors.Is(err, fleet.ErrInvalidBootstrapRequest) && !errors.Is(err, fleet.ErrBootstrapRejected) {
			h.logger.Error("node bootstrap failed", "component", "scalecp", "error", err)
		}
		return h.reply(client, reply, bootstrapErrorResponse(err))
	}

	h.logger.Info("node bootstrap acknowledged", "component", "scalecp", "node_id", out.NodeID, "region", out.Region)
	return h.reply(client, reply, &pb.NodeBootstrapResponse{NodeId: out.NodeID, Region: out.Region})
}

func bootstrapErrorResponse(err error) *pb.NodeBootstrapResponse {
	switch {
	case err == nil:
		return &pb.NodeBootstrapResponse{}
	case errors.Is(err, fleet.ErrInvalidBootstrapRequest):
		return &pb.NodeBootstrapResponse{Error: fleet.ErrInvalidBootstrapRequest.Error()}
	case errors.Is(err, fleet.ErrBootstrapRejected):
		return &pb.NodeBootstrapResponse{Error: fleet.ErrBootstrapRejected.Error()}
	default:
		return &pb.NodeBootstrapResponse{Error: "internal error"}
	}
}

func validateBootstrapRequest(req *pb.NodeBootstrapRequest) (fleet.BootstrapInput, error) {
	if req == nil {
		return fleet.BootstrapInput{}, fleet.ErrInvalidBootstrapRequest
	}

	token := strings.TrimSpace(req.GetBootstrapToken())
	if token == "" {
		return fleet.BootstrapInput{}, fleet.ErrInvalidBootstrapRequest
	}

	bootID := strings.TrimSpace(req.GetBootId())
	if bootID == "" {
		return fleet.BootstrapInput{}, fleet.ErrInvalidBootstrapRequest
	}

	totalCores, ok := uint32ToInt32(req.GetTotalCores())
	if !ok {
		return fleet.BootstrapInput{}, fleet.ErrInvalidBootstrapRequest
	}

	totalRamMB, ok := uint64ToInt64(req.GetTotalRamMb())
	if !ok {
		return fleet.BootstrapInput{}, fleet.ErrInvalidBootstrapRequest
	}

	totalDiskMB, ok := uint64ToInt64(req.GetTotalDiskMb())
	if !ok {
		return fleet.BootstrapInput{}, fleet.ErrInvalidBootstrapRequest
	}

	return fleet.BootstrapInput{
		Token:       token,
		TotalCores:  totalCores,
		TotalRamMb:  totalRamMB,
		TotalDiskMb: totalDiskMB,
	}, nil
}

func uint32ToInt32(v uint32) (int32, bool) {
	if v == 0 || v > maxInt32Uint32 {
		return 0, false
	}
	return int32(v), true
}

func uint64ToInt64(v uint64) (int64, bool) {
	if v == 0 || v > maxInt64Uint64 {
		return 0, false
	}
	return int64(v), true
}

func (h *BootstrapHandler) reply(client *nats.Client, subject string, resp *pb.NodeBootstrapResponse) error {
	return client.PublishProto(subject, resp)
}
