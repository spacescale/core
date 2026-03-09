package state

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	pgstore "github.com/t0gun/spacescale/internal/postgres/gen"
)

const (
	StatusReady    = "ready"
	StatusDraining = "draining"
	StatusDegraded = "degraded"
	StatusOffline  = "offline"

	defaultLastSeenFlushInterval = 30 * time.Second
	persistenceWriteTimeout      = 2 * time.Second
)

var (
	ErrAgentNotFound   = errors.New("agent not found")
	ErrSessionMismatch = errors.New("agent session mismatch")
)

// PersistingAgentStore keeps the heartbeat/session hot-path in memory while
// writing durable identity/presence metadata to Postgres.
type PersistingAgentStore struct {
	queries *pgstore.Queries
	logger  *slog.Logger

	lastSeenFlushInterval time.Duration

	mu        sync.Mutex
	agents    map[string]*liveAgent
	persisted map[string]persistedAgentSnapshot
	metadata  map[string]persistedAgentMetadata
}

type liveAgent struct {
	Hostname         string
	Status           string
	SessionID        string
	LastSeen         time.Time
	RunningWorkloads uint32
	QueuedWorkloads  uint32
	Capabilities     []string
	Version          string
}

type persistedAgentSnapshot struct {
	Status    string
	SessionID string
	LastSeen  time.Time
}

type persistedAgentMetadata struct {
	Name         string
	Capabilities []string
}

// NewPersistingAgentStore builds the production store with DB persistence
// enabled and a throttled last_seen flush cadence.
func NewPersistingAgentStore(queries *pgstore.Queries, lastSeenFlushInterval time.Duration, logger *slog.Logger) (*PersistingAgentStore, error) {
	if queries == nil {
		return nil, errors.New("persisting agent store requires non-nil queries")
	}
	return newAgentStore(queries, lastSeenFlushInterval, logger), nil
}

// NewTransientAgentStore builds a memory-only store used by fast unit tests.
func NewTransientAgentStore(lastSeenFlushInterval time.Duration, logger *slog.Logger) *PersistingAgentStore {
	return newAgentStore(nil, lastSeenFlushInterval, logger)
}

func newAgentStore(queries *pgstore.Queries, lastSeenFlushInterval time.Duration, logger *slog.Logger) *PersistingAgentStore {
	if logger == nil {
		logger = slog.Default()
	}
	if lastSeenFlushInterval <= 0 {
		lastSeenFlushInterval = defaultLastSeenFlushInterval
	}

	return &PersistingAgentStore{
		queries:               queries,
		logger:                logger,
		lastSeenFlushInterval: lastSeenFlushInterval,
		agents:                make(map[string]*liveAgent),
		persisted:             make(map[string]persistedAgentSnapshot),
		metadata:              make(map[string]persistedAgentMetadata),
	}
}

func (s *PersistingAgentStore) Register(agentID, sessionID, hostname, version string, capabilities []string, now time.Time) {
	agentKey := strings.TrimSpace(agentID)
	if agentKey == "" {
		return
	}

	normalizedSessionID := strings.TrimSpace(sessionID)
	agentName := strings.TrimSpace(hostname)
	if agentName == "" {
		agentName = agentKey
	}
	normalizedCaps := append([]string(nil), capabilities...)
	nowUTC := now.UTC()

	s.mu.Lock()
	s.agents[agentKey] = &liveAgent{
		Hostname:         agentName,
		Status:           StatusReady,
		SessionID:        normalizedSessionID,
		LastSeen:         nowUTC,
		Capabilities:     append([]string(nil), normalizedCaps...),
		Version:          strings.TrimSpace(version),
		RunningWorkloads: 0,
		QueuedWorkloads:  0,
	}
	s.metadata[agentKey] = persistedAgentMetadata{
		Name:         agentName,
		Capabilities: append([]string(nil), normalizedCaps...),
	}
	s.mu.Unlock()

	if s.hasPersistence() {
		if err := s.upsertAgent(agentKey, agentName, StatusReady, normalizedCaps, normalizedSessionID, nowUTC); err != nil {
			s.logger.Warn("agent register persistence failed", "agent_key", agentKey, "error", err)
			return
		}
	}

	s.markPersisted(agentKey, persistedAgentSnapshot{
		Status:    StatusReady,
		SessionID: normalizedSessionID,
		LastSeen:  nowUTC,
	})
}

func (s *PersistingAgentStore) UpdateHeartbeat(agentID, sessionID, status string, running, queued uint32, now time.Time) error {
	agentKey := strings.TrimSpace(agentID)
	normalizedSessionID := strings.TrimSpace(sessionID)
	normalizedStatus := normalizeStatus(status)
	nowUTC := now.UTC()

	s.mu.Lock()
	agent, ok := s.agents[agentKey]
	if !ok {
		s.mu.Unlock()
		return ErrAgentNotFound
	}
	if agent.SessionID != normalizedSessionID {
		s.mu.Unlock()
		return ErrSessionMismatch
	}

	agent.Status = normalizedStatus
	agent.LastSeen = nowUTC
	agent.RunningWorkloads = running
	agent.QueuedWorkloads = queued
	s.mu.Unlock()

	if !s.shouldPersistHeartbeat(agentKey, normalizedSessionID, normalizedStatus, nowUTC) {
		return nil
	}

	if s.hasPersistence() {
		rows, err := s.updateAgentLastSeenAndStatus(agentKey, normalizedStatus, normalizedSessionID, nowUTC)
		if err != nil {
			s.logger.Warn("agent heartbeat persistence failed", "agent_key", agentKey, "error", err)
			return nil
		}

		if rows == 0 {
			meta := s.loadMetadata(agentKey)
			if meta.Name == "" {
				meta.Name = agentKey
			}
			if err := s.upsertAgent(agentKey, meta.Name, normalizedStatus, meta.Capabilities, normalizedSessionID, nowUTC); err != nil {
				s.logger.Warn("agent heartbeat upsert fallback failed", "agent_key", agentKey, "error", err)
				return nil
			}
		}
	}

	s.markPersisted(agentKey, persistedAgentSnapshot{
		Status:    normalizedStatus,
		SessionID: normalizedSessionID,
		LastSeen:  nowUTC,
	})

	return nil
}

func (s *PersistingAgentStore) MarkOffline(agentID, sessionID string, now time.Time) {
	agentKey := strings.TrimSpace(agentID)
	normalizedSessionID := strings.TrimSpace(sessionID)
	if agentKey == "" || normalizedSessionID == "" {
		return
	}

	nowUTC := now.UTC()

	s.mu.Lock()
	agent, ok := s.agents[agentKey]
	if !ok || agent.SessionID != normalizedSessionID {
		s.mu.Unlock()
		return
	}
	agent.Status = StatusOffline
	agent.LastSeen = nowUTC
	s.mu.Unlock()

	if s.hasPersistence() {
		rows, err := s.markAgentOffline(agentKey, normalizedSessionID, nowUTC)
		if err != nil {
			s.logger.Warn("agent offline persistence failed", "agent_key", agentKey, "error", err)
			return
		}

		if rows == 0 {
			meta := s.loadMetadata(agentKey)
			if meta.Name == "" {
				meta.Name = agentKey
			}
			if err := s.upsertAgent(agentKey, meta.Name, StatusOffline, meta.Capabilities, normalizedSessionID, nowUTC); err != nil {
				s.logger.Warn("agent offline upsert fallback failed", "agent_key", agentKey, "error", err)
				return
			}
		}
	}

	s.markPersisted(agentKey, persistedAgentSnapshot{
		Status:    StatusOffline,
		SessionID: normalizedSessionID,
		LastSeen:  nowUTC,
	})
}

func (s *PersistingAgentStore) SweepOffline(leaseTTL time.Duration, now time.Time) int {
	if leaseTTL <= 0 {
		return 0
	}

	cutoff := now.UTC().Add(-leaseTTL)

	s.mu.Lock()
	changed := 0
	for _, agent := range s.agents {
		if (agent.LastSeen.Before(cutoff) || agent.LastSeen.Equal(cutoff)) && agent.Status != StatusOffline {
			agent.Status = StatusOffline
			changed++
		}
	}
	for agentKey, snapshot := range s.persisted {
		if snapshot.LastSeen.Before(cutoff) || snapshot.LastSeen.Equal(cutoff) {
			snapshot.Status = StatusOffline
			s.persisted[agentKey] = snapshot
		}
	}
	s.mu.Unlock()

	if s.hasPersistence() {
		if _, err := s.markStaleAgentsOffline(cutoff); err != nil {
			s.logger.Warn("stale agent offline sweep persistence failed", "error", err)
		}
	}

	return changed
}

func (s *PersistingAgentStore) shouldPersistHeartbeat(agentKey, sessionID, status string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	snapshot, ok := s.persisted[agentKey]
	if !ok {
		return true
	}
	if snapshot.SessionID != sessionID {
		return true
	}
	if snapshot.Status != status {
		return true
	}

	return now.Sub(snapshot.LastSeen) >= s.lastSeenFlushInterval
}

func (s *PersistingAgentStore) loadMetadata(agentKey string) persistedAgentMetadata {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, ok := s.metadata[agentKey]
	if !ok {
		return persistedAgentMetadata{}
	}

	return persistedAgentMetadata{
		Name:         meta.Name,
		Capabilities: append([]string(nil), meta.Capabilities...),
	}
}

func (s *PersistingAgentStore) markPersisted(agentKey string, snapshot persistedAgentSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.persisted[agentKey] = snapshot
}

func (s *PersistingAgentStore) hasPersistence() bool {
	return s.queries != nil
}

func (s *PersistingAgentStore) upsertAgent(agentKey, name, status string, capabilities []string, sessionID string, lastSeenAt time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), persistenceWriteTimeout)
	defer cancel()

	return s.queries.UpsertAgent(ctx, pgstore.UpsertAgentParams{
		AgentKey:      agentKey,
		Name:          name,
		Status:        status,
		Capabilities:  append([]string(nil), capabilities...),
		LastSessionID: sessionID,
		LastSeenAt:    lastSeenAt,
	})
}

func (s *PersistingAgentStore) updateAgentLastSeenAndStatus(agentKey, status, sessionID string, lastSeenAt time.Time) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), persistenceWriteTimeout)
	defer cancel()

	return s.queries.UpdateAgentLastSeenAndStatus(ctx, pgstore.UpdateAgentLastSeenAndStatusParams{
		AgentKey:      agentKey,
		Status:        status,
		LastSeenAt:    lastSeenAt,
		LastSessionID: sessionID,
	})
}

func (s *PersistingAgentStore) markAgentOffline(agentKey, sessionID string, lastSeenAt time.Time) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), persistenceWriteTimeout)
	defer cancel()

	return s.queries.MarkAgentOffline(ctx, pgstore.MarkAgentOfflineParams{
		AgentKey:      agentKey,
		LastSeenAt:    lastSeenAt,
		LastSessionID: sessionID,
	})
}

func (s *PersistingAgentStore) markStaleAgentsOffline(cutoff time.Time) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), persistenceWriteTimeout)
	defer cancel()

	return s.queries.MarkStaleAgentsOffline(ctx, cutoff)
}

func normalizeStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case StatusReady:
		return StatusReady
	case StatusDraining:
		return StatusDraining
	case StatusDegraded:
		return StatusDegraded
	case StatusOffline:
		return StatusOffline
	default:
		return StatusReady
	}
}
