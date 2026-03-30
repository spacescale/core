package broker

import (
	"context"
	"errors"
	"log/slog"

	"github.com/spacescale/core/internal/scalecp/service"
	nodesvc "github.com/spacescale/core/internal/scalecp/service/node"
	"github.com/spacescale/core/internal/shared/nats"
	scalepb "github.com/spacescale/core/internal/shared/pb/v1"
)

type NodeBroker struct {
	registrar *nodesvc.Registrar // Manages node registration and hardware specs from the database
	logger    *slog.Logger
}

func NewNodeBroker(services service.NodeServices, logger *slog.Logger) *NodeBroker {
	return &NodeBroker{registrar: services.Registrar, logger: logger}
}

func (b *NodeBroker) Register(ctx context.Context, client *nats.Client) error {
	if _, err := client.QueueSubscribe(nats.SubjectNodeBootstrap, nats.QueueNodeBootstrap, func(msg *nats.Msg) error {
		return b.handleBootstrap(ctx, client, msg)
	}); err != nil {
		return err
	}

	return nil
}

func (b *NodeBroker) handleBootstrap(ctx context.Context, client *nats.Client, msg *nats.Msg) error {
	reply := msg.Reply
	if reply == "" {
		return errors.New("bootstrap request is missing reply subject")
	}

	req := &scalepb.NodeBootstrapRequest{}
	if err := nats.UnmarshalProto(msg, req); err != nil {
		// respond only to bootstrap but fill the error field only
		return b.replyBootstrap(client, reply, &scalepb.NodeBootstrapResponse{Error: nodesvc.ErrInvalidBootstrapRequest.Error()})
	}

	out, err := b.registrar.Register(ctx, req)
	if err != nil {
		if !errors.Is(err, nodesvc.ErrInvalidBootstrapRequest) && !errors.Is(err, nodesvc.ErrBootstrapRejected) {
			b.logger.Error("node bootstrap failed", "component", "scalecp", "error", err)
		}
		return b.replyBootstrap(client, reply, bootstrapErrorResponse(err))
	}
	b.logger.Info("node bootstrap acknowledged", "component", "scalecp", "node_id", out.NodeID, "region", out.Region)
	return b.replyBootstrap(client, reply, &scalepb.NodeBootstrapResponse{NodeId: out.NodeID, Region: out.Region})
}

func bootstrapErrorResponse(err error) *scalepb.NodeBootstrapResponse {
	switch {
	case err == nil:
		return &scalepb.NodeBootstrapResponse{}
	case errors.Is(err, nodesvc.ErrInvalidBootstrapRequest):
		return &scalepb.NodeBootstrapResponse{Error: nodesvc.ErrInvalidBootstrapRequest.Error()}
	case errors.Is(err, nodesvc.ErrBootstrapRejected):
		return &scalepb.NodeBootstrapResponse{Error: nodesvc.ErrBootstrapRejected.Error()}
	default:
		return &scalepb.NodeBootstrapResponse{Error: "internal error"}
	}
}

func (b *NodeBroker) replyBootstrap(client *nats.Client, subject string, resp *scalepb.NodeBootstrapResponse) error {
	return client.PublishProto(subject, resp)
}
