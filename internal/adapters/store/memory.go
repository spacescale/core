package store

import (
	"context"
	"sync"

	"github.com/t0gun/paas/internal/contracts"
	"github.com/t0gun/paas/internal/domain"
)

type MemoryStore struct {
	mu     sync.RWMutex
	byID   map[string]domain.App
	byName map[string]domain.App

	depByID     map[string]domain.Deployment
	depsByAPPID map[string][]domain.Deployment
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		byID:   make(map[string]domain.App),
		byName: make(map[string]domain.App),

		depByID:     make(map[string]domain.Deployment),
		depsByAPPID: make(map[string][]domain.Deployment),
	}
}

func (s *MemoryStore) CreateApp(ctx context.Context, app domain.App) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// if name already exists in map
	if _, ok := s.byName[app.Name]; ok {
		return contracts.ErrConflict
	}

	s.byID[app.ID] = app
	s.byName[app.Name] = app
	return nil
}

func (s *MemoryStore) GetAppByID(ctx context.Context, id string) (domain.App, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	app, ok := s.byID[id]
	if !ok {
		return domain.App{}, contracts.ErrNotFound
	}
	return app, nil
}

func (s *MemoryStore) GetAppByName(ctx context.Context, name string) (domain.App, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	app, ok := s.byName[name]
	if !ok {
		return domain.App{}, contracts.ErrNotFound
	}
	return app, nil
}

func (s *MemoryStore) ListApps(ctx context.Context) ([]domain.App, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]domain.App, 0, len(s.byID))
	for _, a := range s.byID {
		out = append(out, a)
	}
	return out, nil
}

func (s *MemoryStore) CreateDeployment(ctx context.Context, dep domain.Deployment) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// optional safety: app must exit
	if _, ok := s.byID[dep.AppID]; !ok {
		return contracts.ErrNotFound
	}

	s.depByID[dep.ID] = dep
	s.depsByAPPID[dep.AppID] = append(s.depsByAPPID[dep.AppID], dep)
	return nil
}

func (s *MemoryStore) GetDeploymentByID(ctx context.Context, id string) (domain.Deployment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	dep, ok := s.depByID[id]
	if !ok {
		return domain.Deployment{}, contracts.ErrNotFound
	}

	return dep, nil
}

func (s *MemoryStore) ListDeploymentsByAppID(ctx context.Context, appID string) ([]domain.Deployment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	deps, ok := s.depsByAPPID[appID]
	if !ok {
		return []domain.Deployment{}, nil
	}

	// return a copy so caller cant mutate internal slice
	out := make([]domain.Deployment, len(deps))
	copy(out, deps)
	return out, nil
}
