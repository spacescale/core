// Copyright (c) 2026 SpaceScale Systems Inc. All rights reserved.

package node

import (
	"context"
	"log/slog"
	"time"

	"github.com/spacescale/core/internal/shared/nats"
	"github.com/spacescale/core/internal/shared/pb/v1"
)

const HeartbeatInterval = 5 * time.Second

func Heartbeater(ctx context.Context, kv nats.KeyValue, nodeID, bootID string, logger *slog.Logger) {
	key := nats.NodeHeartbeatKey(nodeID)
	seqNo := uint64(1)

	publishHeartbeat(ctx, kv, key, nodeID, bootID, seqNo, logger)

	ticker := time.NewTicker(HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			seqNo++
			publishHeartbeat(ctx, kv, key, nodeID, bootID, seqNo, logger)
		}
	}
}

func publishHeartbeat(ctx context.Context, kv nats.KeyValue, key, nodeID, bootID string, seqNo uint64, logger *slog.Logger) {
	hb := &pb.NodeHeartbeat{
		NodeId:         nodeID,
		SeqNo:          seqNo,
		BootId:         bootID,
		SentAtUnixNano: time.Now().UnixNano(),
	}

	if _, err := nats.PutProtoKV(ctx, kv, key, hb); err != nil {
		logger.Warn("failed to write heartbeat to KV", "node_id", nodeID, "error", err)
	}
}
