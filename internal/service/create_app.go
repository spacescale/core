package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/t0gun/paas/internal/contracts"
	"github.com/t0gun/paas/internal/domain"
)

type AppService struct {
	store   contracts.Store
	runtime contracts.Runtime
}

func NewAppService(store contracts.Store) *AppService {
	return &AppService{store: store}
}

func NewAppServiceWithRuntime(store contracts.Store, rt contracts.Runtime) *AppService {
	return &AppService{store: store, runtime: rt}
}

type CreateAppParams struct {
	Name  string
	Image string
	Port  int
}

func (s *AppService) CreateApp(ctx context.Context, p CreateAppParams) (domain.App, error) {
	app, err := domain.NewApp(domain.NewAppParams{
		Name:  p.Name,
		Image: p.Image,
		Port:  p.Port,
	})
	if err != nil {
		return domain.App{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	if err := s.store.CreateApp(ctx, app); err != nil {
		if errors.Is(err, contracts.ErrConflict) {
			return domain.App{}, ErrConflict
		}
		return domain.App{}, err
	}

	return app, nil
}
