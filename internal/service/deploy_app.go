package service

import (
	"context"
	"errors"
	"fmt"
	"time"

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

func (s *AppService) ProcessNextDeployment(ctx context.Context) (domain.Deployment, error) {
	if s.runtime == nil {
		return domain.Deployment{}, ErrNoRuntime
	}

	dep, err := s.store.TakeNextQueuedDeployment(ctx)
	if err != nil {
		if errors.Is(err, contracts.ErrNotFound) {
			return domain.Deployment{}, ErrNoWork
		}
		return domain.Deployment{}, err
	}

	// Building
	dep.Status = domain.DeploymentStatusBuilding
	dep.UpdatedAt = time.Now()
	if err := s.store.UpdateDeployment(ctx, dep); err != nil {
		return domain.Deployment{}, err
	}

	app, err := s.store.GetAppByID(ctx, dep.AppID)
	if err != nil {
		msg := err.Error()
		dep.Status = domain.DeploymentStatusFailed
		dep.Error = &msg
		dep.UpdatedAt = time.Now().UTC()
		_ = s.store.UpdateDeployment(ctx, dep)
		return dep, fmt.Errorf("runtime deploy failed: %w", err)
	}

	url, err := s.runtime.Deploy(ctx, app)
	if err != nil {
		msg := err.Error()
		dep.Status = domain.DeploymentStatusFailed
		dep.Error = &msg
		dep.UpdatedAt = time.Now().UTC()
		_ = s.store.UpdateDeployment(ctx, dep)
		return dep, fmt.Errorf("runtime deploy failed: %w", err)
	}

	// RUNNING
	dep.Status = domain.DeploymentStatusRunning
	dep.URL = &url
	dep.Error = nil
	dep.UpdatedAt = time.Now().UTC()
	if err := s.store.UpdateDeployment(ctx, dep); err != nil {
		return domain.Deployment{}, err
	}

	return dep, nil

}
