package node

import (
	"context"
	"errors"
	"strings"

	"github.com/spacescale/core/internal/scalecp/db/sqlc"
	scalepb "github.com/spacescale/core/internal/shared/pb/v1"
)

var (
	ErrInvalidHeartbeat = errors.New("invalid node heartbeat")
	ErrUnknownNode      = errors.New("unknown node")
)

type PresenceManager struct {
	queries *sqlc.Queries
}

func NewPresenceManager(queries *sqlc.Queries) *PresenceManager {
	if queries == nil {
		panic("node.NewPresenceManager requires non-nil queries")
	}
	return &PresenceManager{
		queries: queries,
	}
}

func (m *PresenceManager) ApplyHeartbeat(ctx context.Context, hb *scalepb.NodeHeartbeat) error {
	nodeID, bootID, status, statusReason, totalRunningVMs, err := validateHeartbeat(hb)
	if err != nil {
		return err
	}
	rows, err := m.queries.UpdateScaledPresence(ctx, sqlc.UpdateScaledPresenceParams{
		ID:              nodeID,
		BootID:          bootID,
		Status:          status,
		StatusReason:    nullableString(statusReason),
		TotalRunningVms: totalRunningVMs,
	})
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrUnknownNode
	}
	if _, err := m.queries.MarkMetalActiveByNodeID(ctx, nodeID); err != nil {
		return err
	}
	return nil
}

func (m *PresenceManager) MarkOffline(ctx context.Context, nodeID string, reason string) error {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return ErrInvalidHeartbeat
	}
	rows, err := m.queries.MarkScaledOffline(ctx, sqlc.MarkScaledOfflineParams{
		ID:           nodeID,
		StatusReason: nullableString(normalizeStatusReason(reason)),
	})
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrUnknownNode
	}
	return nil
}

func validateHeartbeat(hb *scalepb.NodeHeartbeat) (string, string, string, string, int32, error) {
	if hb == nil {
		return "", "", "", "", 0, ErrInvalidHeartbeat
	}
	nodeID := strings.TrimSpace(hb.GetNodeId())
	if nodeID == "" {
		return "", "", "", "", 0, ErrInvalidHeartbeat
	}
	bootID := strings.TrimSpace(hb.GetBootId())
	if bootID == "" {
		return "", "", "", "", 0, ErrInvalidHeartbeat
	}
	status, ok := normalizeHeartbeatStatus(hb.GetStatus())
	if !ok {
		return "", "", "", "", 0, ErrInvalidHeartbeat
	}
	statusReason := normalizeStatusReason(hb.GetStatusReason())
	if status == "ready" {
		statusReason = ""
	}
	totalRunningVMs, ok := uint32ToInt32Any(hb.GetActiveVms())
	if !ok {
		return "", "", "", "", 0, ErrInvalidHeartbeat
	}
	return nodeID, bootID, status, statusReason, totalRunningVMs, nil
}

func normalizeHeartbeatStatus(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "ready":
		return "ready", true
	case "draining":
		return "draining", true
	case "offline":
		return "offline", true
	case "cordoned":
		return "cordoned", true
	case "overheated":
		return "overheated", true
	default:
		return "", false
	}
}

func normalizeStatusReason(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if len(raw) > 255 {
		return raw[:255]
	}
	return raw
}

func nullableString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func uint32ToInt32Any(v uint32) (int32, bool) {
	if v > maxInt32Uint32 {
		return 0, false
	}
	return int32(v), true
}
