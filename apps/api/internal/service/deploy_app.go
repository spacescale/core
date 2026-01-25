// Service logic for deployments and processing
// This file validates input and loads app data
// It queues deployments and updates status fields
// It calls the runtime to deploy apps
// It records url or error results on deployments

package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/t0gun/spacescale/internal/contracts"
	"github.com/t0gun/spacescale/internal/domain"
)

// DeployAppParams contains the input needed to request a deployment
type DeployAppParams struct {
	AppID string
}

// ListDeploymentsParams identifies which app to list deployments for
type ListDeploymentsParams struct {
	AppID string
}

// DeployApp creates a queued deployment for an app.
func (s *AppService) DeployApp(ctx context.Context, p DeployAppParams) (domain.Deployment, error) {
	if p.AppID == "" {
		return domain.Deployment{}, fmt.Errorf("%w: app id is required", ErrInvalidInput)
	}

	// Ensure the app exists before creating a deployment record
	_, err := s.store.GetAppByID(ctx, p.AppID)
	if err != nil {
		if errors.Is(err, contracts.ErrNotFound) {
			return domain.Deployment{}, ErrNotFound
		}
		return domain.Deployment{}, err
	}

	// Create a queued deployment record
	dep := domain.NewDeployment(p.AppID)
	if err := s.store.CreateDeployment(ctx, dep); err != nil {
		if errors.Is(err, contracts.ErrNotFound) {
			// Store enforces that the app must exist
			return domain.Deployment{}, ErrNotFound
		}
		return domain.Deployment{}, err
	}
	return dep, nil
}

// ProcessNextDeployment runs the next queued deployment.
func (s *AppService) ProcessNextDeployment(ctx context.Context) (domain.Deployment, error) {
	if s.runtime == nil {
		return domain.Deployment{}, ErrNoRuntime
	}

	// Grab the next queued deployment in FIFO order
	dep, err := s.store.TakeNextQueuedDeployment(ctx)
	if err != nil {
		if errors.Is(err, contracts.ErrNotFound) {
			return domain.Deployment{}, ErrNoWork
		}
		return domain.Deployment{}, err
	}

	// Mark deployment as building before interacting with the runtime
	dep.Status = domain.DeploymentStatusBuilding
	dep.UpdatedAt = time.Now()
	if err := s.store.UpdateDeployment(ctx, dep); err != nil {
		return domain.Deployment{}, err
	}

	// Load app data needed for the runtime deployment
	app, err := s.store.GetAppByID(ctx, dep.AppID)
	if err != nil {
		msg := err.Error()
		dep.Status = domain.DeploymentStatusFailed
		dep.Error = &msg
		dep.UpdatedAt = time.Now().UTC()
		_ = s.store.UpdateDeployment(ctx, dep)
		return dep, fmt.Errorf("runtime deploy failed: %w", err)
	}

	// Run the runtime deploy and capture a URL or an error
	url, err := s.runtime.Deploy(ctx, app)
	if err != nil {
		msg := err.Error()
		dep.Status = domain.DeploymentStatusFailed
		dep.Error = &msg
		dep.UpdatedAt = time.Now().UTC()
		_ = s.store.UpdateDeployment(ctx, dep)
		return dep, fmt.Errorf("runtime deploy failed: %w", err)
	}

	// Mark deployment as running with the resolved URL
	dep.Status = domain.DeploymentStatusRunning
	dep.URL = url
	dep.Error = nil
	dep.UpdatedAt = time.Now().UTC()
	if err := s.store.UpdateDeployment(ctx, dep); err != nil {
		return domain.Deployment{}, err
	}

	return dep, nil

}

// ListDeployments returns deployments for an app.
func (s *AppService) ListDeployments(ctx context.Context, p ListDeploymentsParams) ([]domain.Deployment, error) {
	if p.AppID == "" {
		return nil, ErrInvalidInput
	}

	// Ensure the app exists so missing apps return a not found error
	if _, err := s.store.GetAppByID(ctx, p.AppID); err != nil {
		if errors.Is(err, contracts.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	deps, err := s.store.ListDeploymentsByAppID(ctx, p.AppID)
	if err != nil {
		return nil, err
	}
	return deps, nil
}
