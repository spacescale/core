package broker

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/spacescale/core/internal/scalecp/service"
	nodesvc "github.com/spacescale/core/internal/scalecp/service/node"
	"github.com/spacescale/core/internal/shared/nats"
	scalepb "github.com/spacescale/core/internal/shared/pb/v1"
)

type NodeBroker struct {
	registrar  *nodesvc.Registrar // Manages node registration and hardware specs from the database
	logger     *slog.Logger
	heartbeats nats.KeyValue
}

func NewNodeBroker(services service.NodeServices, logger *slog.Logger) *NodeBroker {
	if services.Registrar == nil {
		panic("broker.NewNodeBroker requires non-nil registrar")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &NodeBroker{registrar: services.Registrar, logger: logger}
}

func (b *NodeBroker) Register(ctx context.Context, client *nats.Client) error {
	if client == nil {
		return errors.New("nats client is required")
	}
	// ensure  key value bucket for heartbeat is created
	kv, err := client.EnsureNodeHeartbeatKV(ctx)
	if err != nil {
		return err
	}
	b.heartbeats = kv // store kv in node broker

	// if new servers join they shout to this worker pool. Any CP can take the boostrap request and respond to it
	if _, err := client.QueueSubscribe(nats.SubjectNodeBootstrap, nats.QueueNodeBootstrap, func(msg *nats.Msg) error {
		return b.handleBootstrap(ctx, client, msg)
	}); err != nil {
		return err
	}
	// once servers are registered, Any CP can start listening to heartbeats from this  Queued group as well
	if _, err := client.QueueSubscribe(nats.SubjectNodeHeartbeat, nats.QueueNodeHeartbeat, func(msg *nats.Msg) error {
		return b.handleHeartbeat(ctx, msg)
	}); err != nil {
		return err
	}

	return nil
}

func (b *NodeBroker) handleBootstrap(ctx context.Context, client *nats.Client, msg *nats.Msg) error {
	if msg == nil {
		return errors.New("nats message is required")
	}

	reply := strings.TrimSpace(msg.Reply)
	if reply == "" {
		return errors.New("bootstrap request is missing reply subject")
	}

	req := &scalepb.NodeBootstrapRequest{}
	if err := nats.UnmarshalProto(msg, req); err != nil {
		// respond only to bootstrap but fill the error field only
		return b.replyBootstrap(client, reply, &scalepb.NodeBootstrapResponse{Error: nodesvc.ErrInvalidBootstrapRequest.Error()})
	}

	// register node after boostrap completion
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

func (b *NodeBroker) handleHeartbeat(ctx context.Context, msg *nats.Msg) error {
	if msg == nil {
		return errors.New("nats message is required")
	}
	if b.heartbeats == nil {
		return errors.New("heartbeat key value store is not initialized")
	}
	hb := &scalepb.NodeHeartbeat{}
	// the new heart bet marshaled here
	if err := nats.UnmarshalProto(msg, hb); err != nil {
		return err

	}
	// build the nodeKey that will be use to get heartbeat from KV
	key, err := nats.NodeHeartbeatKey(strings.TrimSpace(hb.GetNodeId()))
	if err != nil {
		return err
	}

	prev := &scalepb.NodeHeartbeat{}
	// check for existing heartbeats with that same key
	found, _, err := nats.GetProtoKV(ctx, b.heartbeats, key, prev)
	if err != nil {
		return err
	}

	// if new  heartbeat came in and its stale due to drop network or something, we drop it
	if found && isStaleHeartbeat(prev, hb) {
		return nil
	}
	_, err = nats.PutProtoKV(ctx, b.heartbeats, key, hb)
	if err != nil {
		return err
	}

	if !found {
		if err := b.registrar.MarkMetalActive(ctx, hb.GetNodeId()); err != nil {
			return err
		}
		b.logger.Info("node activated", "component", "scalecp", "node_id", hb.GetNodeId())
	}

	return nil
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
	if client == nil {
		return errors.New("nats client is required")
	}
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return errors.New("nats reply subject is required")
	}
	if resp == nil {
		return errors.New("bootstrap response is required")
	}
	return client.PublishProto(subject, resp)
}

func isStaleHeartbeat(prev, next *scalepb.NodeHeartbeat) bool {
	if prev == nil || next == nil {
		return false
	}

	if strings.TrimSpace(prev.GetBootId()) != strings.TrimSpace(next.GetBootId()) {
		return false
	}

	if prev.GetSeqNo() == 0 || next.GetSeqNo() == 0 {
		return false
	}
	return next.GetSeqNo() <= prev.GetSeqNo()
}
