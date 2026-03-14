package state

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPersistingAgentStoreRegisterAndHeartbeatUpdate(t *testing.T) {
	store := NewTransientAgentStore(30*time.Second, nil)
	registeredAt := time.Now().Add(-30 * time.Second)
	heartbeatAt := time.Now()

	store.Register("agent-1", "session-1", "node-a", "v1", []string{"firecracker"}, registeredAt)
	err := store.UpdateHeartbeat("agent-1", "session-1", StatusDegraded, 3, 2, heartbeatAt)
	require.NoError(t, err)

	agent, ok := store.agents["agent-1"]
	require.True(t, ok)
	require.Equal(t, "node-a", agent.Hostname)
	require.Equal(t, "v1", agent.Version)
	require.Equal(t, StatusDegraded, agent.Status)
	require.Equal(t, uint32(3), agent.RunningWorkloads)
	require.Equal(t, uint32(2), agent.QueuedWorkloads)
	require.WithinDuration(t, heartbeatAt.UTC(), agent.LastSeen, time.Millisecond)
}

func TestPersistingAgentStoreUpdateHeartbeatMismatchHandling(t *testing.T) {
	store := NewTransientAgentStore(30*time.Second, nil)
	store.Register("agent-1", "session-1", "node-a", "v1", nil, time.Now())

	err := store.UpdateHeartbeat("agent-1", "wrong-session", StatusReady, 0, 0, time.Now())
	require.True(t, errors.Is(err, ErrSessionMismatch))

	err = store.UpdateHeartbeat("missing-agent", "session-1", StatusReady, 0, 0, time.Now())
	require.True(t, errors.Is(err, ErrAgentNotFound))
}

func TestPersistingAgentStoreSweepOfflineMarksExpiredAgents(t *testing.T) {
	store := NewTransientAgentStore(30*time.Second, nil)
	now := time.Now().UTC()

	store.Register("agent-old", "session-old", "node-old", "v1", nil, now.Add(-2*time.Minute))
	store.Register("agent-fresh", "session-fresh", "node-fresh", "v1", nil, now.Add(-10*time.Second))

	changed := store.SweepOffline(30*time.Second, now)
	require.Equal(t, 1, changed)
	require.Equal(t, StatusOffline, store.agents["agent-old"].Status)
	require.Equal(t, StatusReady, store.agents["agent-fresh"].Status)
}
