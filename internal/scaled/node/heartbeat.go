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
	identity.Region = strings.TrimSpace(identity.Region)
	bootID = strings.TrimSpace(bootID)
	if identity.NodeID == "" || identity.Region == "" || bootID == "" {
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
		NodeId:              identity.NodeID,
		Region:              identity.Region,
		SeqNo:               seqNo,
		BootId:              bootID,
		SentAtUnixNano:      time.Now().UTC().UnixNano(),
		SharedVcpuAvailable: 0,
		PinnedCpusFree:      0,
		AllocatableMemMb:    0,
		AllocatableDiskMb:   0,
		ActiveVms:           0,
		CpuPsiSome_10S:      0,
		IoPsiSome_10S:       0,
		CpuTempMaxC:         0,
		OomKillsSinceBoot:   0,
		Status:              "ready",
		StatusReason:        "",
		NetworkOk:           true,
		NetUtilizationPct:   0,
	}

	return client.PublishProto(nats.SubjectNodeHeartbeat, hb)
}
