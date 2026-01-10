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
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		byID:   make(map[string]domain.App),
		byName: make(map[string]domain.App),
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