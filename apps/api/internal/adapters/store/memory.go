package store

import (
	"context"
	"sync"

	"github.com/t0gun/paas/internal/contracts"
	"github.com/t0gun/paas/internal/domain"
)

// MemoryStore is an in-memory implementation of contracts.Store.
// It exists for local dev and tests.
//
//   - We keep "indexes" (maps) to support different query patterns.
//   - For deployments, we store the full Deployment only once (deploymentByID).
//     deploymentIDsByAppID is an index of IDs, not full objects, so we avoid
//     duplicated copies that can get out of sync during updates.
type MemoryStore struct {
	mu        sync.RWMutex
	appByID   map[string]domain.App
	appByName map[string]domain.App

	deploymentByID       map[string]domain.Deployment
	deploymentIDsByAppID map[string][]string
	queuedDeploymentIDs  []string
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		appByID:   make(map[string]domain.App),
		appByName: make(map[string]domain.App),

		deploymentByID:       make(map[string]domain.Deployment),
		deploymentIDsByAppID: make(map[string][]string),
		queuedDeploymentIDs:  make([]string, 0),
	}
}

func (s *MemoryStore) CreateApp(ctx context.Context, app domain.App) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Enforce unique app names.
	if _, ok := s.appByName[app.Name]; ok {
		return contracts.ErrConflict
	}

	s.appByID[app.ID] = app
	s.appByName[app.Name] = app
	return nil
}

func (s *MemoryStore) GetAppByID(ctx context.Context, id string) (domain.App, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	app, ok := s.appByID[id]
	if !ok {
		return domain.App{}, contracts.ErrNotFound
	}
	return app, nil
}

func (s *MemoryStore) GetAppByName(ctx context.Context, name string) (domain.App, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	app, ok := s.appByName[name]
	if !ok {
		return domain.App{}, contracts.ErrNotFound
	}
	return app, nil
}

func (s *MemoryStore) ListApps(ctx context.Context) ([]domain.App, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]domain.App, 0, len(s.appByID))
	for _, a := range s.appByID {
		out = append(out, a)
	}
	return out, nil
}
func (s *MemoryStore) CreateDeployment(ctx context.Context, dep domain.Deployment) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Safety: deployment must reference an existing app.
	_, appExists := s.appByID[dep.AppID]
	if !appExists {
		return contracts.ErrNotFound
	}

	// Store deployment once (source of truth).
	s.deploymentByID[dep.ID] = dep

	// Index under app for history/listing (IDs only, preserves create order).
	currentIDs := s.deploymentIDsByAppID[dep.AppID] // nil is fine
	currentIDs = append(currentIDs, dep.ID)
	s.deploymentIDsByAppID[dep.AppID] = currentIDs

	// Enqueue for worker if queued.
	if dep.Status == domain.DeploymentStatusQueued {
		s.queuedDeploymentIDs = append(s.queuedDeploymentIDs, dep.ID)
	}

	return nil
}

func (s *MemoryStore) GetDeploymentByID(ctx context.Context, id string) (domain.Deployment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dep, ok := s.deploymentByID[id]
	if !ok {
		return domain.Deployment{}, contracts.ErrNotFound
	}
	return dep, nil
}

func (s *MemoryStore) ListDeploymentsByAppID(ctx context.Context, appID string) ([]domain.Deployment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := s.deploymentIDsByAppID[appID]
	if len(ids) == 0 {
		return []domain.Deployment{}, nil
	}

	out := make([]domain.Deployment, 0, len(ids))

	// Preserve create order by iterating IDs in the order we appended them.
	for _, depID := range ids {
		dep, ok := s.deploymentByID[depID]
		if !ok {
			// Defensive: history index might contain stale IDs if there was a bug earlier.
			// We skip instead of failing the whole list call.
			continue
		}
		out = append(out, dep)
	}

	return out, nil
}

// TakeNextQueuedDeployment returns and removes the next QUEUED deployment from the FIFO queue.
// We defensively skip stale queue entries (missing deployment) and skip deployments that are
// no longer QUEUED (e.g. already processed but ID still remained in queue).
func (s *MemoryStore) TakeNextQueuedDeployment(ctx context.Context) (domain.Deployment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for len(s.queuedDeploymentIDs) > 0 {
		nextID := s.queuedDeploymentIDs[0]
		s.queuedDeploymentIDs = s.queuedDeploymentIDs[1:]

		dep, exists := s.deploymentByID[nextID]
		if !exists {
			// stale queue entry
			continue
		}

		// Only return deployments that are still eligible.
		if dep.Status != domain.DeploymentStatusQueued {
			continue
		}

		return dep, nil
	}

	return domain.Deployment{}, contracts.ErrNotFound
}

// UpdateDeployment updates the stored deployment source of truth
// Because our app-history index stores only IDs, we do NOT need to update any slices here.
func (s *MemoryStore) UpdateDeployment(ctx context.Context, dep domain.Deployment) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, exists := s.deploymentByID[dep.ID]
	if !exists {
		return contracts.ErrNotFound
	}

	// Source of truth update only.
	s.deploymentByID[dep.ID] = dep
	return nil
}

// Compile-time check: ensure MemoryStore implements the Store contract.
var _ contracts.Store = (*MemoryStore)(nil)
