package node

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/spacescale/core/internal/shared/nats"
	scalepb "github.com/spacescale/core/internal/shared/pb/v1"
)

var errInvalidHeartbeat = errors.New("invalid node heartbeat")

const HeartbeatInterval = 5 * time.Second

func RunHeartbeatLoop(ctx context.Context, client *nats.Client, identity Identity, bootID string, logger *slog.Logger) error {
	if client == nil {
		return errors.New("nats client is required")
	}
	if logger == nil {
		logger = slog.Default()
	}

	identity.NodeID = strings.TrimSpace(identity.NodeID)
	bootID = strings.TrimSpace(bootID)
	if identity.NodeID == "" || bootID == "" {
		return errInvalidHeartbeat
	}

	seqNo := uint64(1)
	if err := publishHeartbeat(client, identity, bootID, seqNo); err != nil {
		return err
	}
	ticker := time.NewTicker(HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil

		case <-ticker.C:
			seqNo++
			if err := publishHeartbeat(client, identity, bootID, seqNo); err != nil {
				logger.Warn("publish heartbeat failed", "component", "scaled", "node_id", identity.NodeID, "error", err)
			}
		}
	}
}

func publishHeartbeat(client *nats.Client, identity Identity, bootID string, seqNo uint64) error {
	hb := &scalepb.NodeHeartbeat{
		NodeId:         identity.NodeID,
		SeqNo:          seqNo,
		BootId:         bootID,
		SentAtUnixNano: time.Now().UTC().UnixNano(),
	}

	return client.PublishProto(nats.SubjectNodeHeartbeat, hb)
}
