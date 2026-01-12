package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/t0gun/paas/internal/contracts"
	"github.com/t0gun/paas/internal/domain"
)

type DeployAppParams struct {
	AppID string
}

func (s *AppService) DeployApp(ctx context.Context, p DeployAppParams) (domain.Deployment, error) {
	if p.AppID == "" {
		return domain.Deployment{}, fmt.Errorf("%w: app id is required", ErrInvalidInput)
	}

	_, err := s.store.GetAppByID(ctx, p.AppID)
	if err != nil {
		if errors.Is(err, contracts.ErrNotFound) {
			return domain.Deployment{}, ErrNotFound
		}
		return domain.Deployment{}, err
	}

	dep := domain.NewDeployment(p.AppID)
	if err := s.store.CreateDeployment(ctx, dep); err != nil {
		if errors.Is(err, contracts.ErrNotFound) {
			// store enforces "app must exists"
			return domain.Deployment{}, ErrNotFound
		}
		return domain.Deployment{}, ErrNotFound
	}
	return dep, nil
}
