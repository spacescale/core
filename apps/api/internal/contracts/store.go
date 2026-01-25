// Store contract definitions for persistence adapters.
package contracts

import (
	"context"

	"github.com/t0gun/spacescale/internal/domain"
)

// Store defines persistence operations for apps and deployments.
type Store interface {
	// CreateApp persists a new app.
	CreateApp(ctx context.Context, app domain.App) error
	// GetAppByID fetches an app by its id.
	GetAppByID(ctx context.Context, id string) (domain.App, error)
	// GetAppByName fetches an app by its name.
	GetAppByName(ctx context.Context, name string) (domain.App, error)
	// ListApps returns all apps.
	ListApps(ctx context.Context) ([]domain.App, error)

	// CreateDeployment persists a new deployment.
	CreateDeployment(ctx context.Context, dep domain.Deployment) error
	// GetDeploymentByID fetches a deployment by its id.
	GetDeploymentByID(ctx context.Context, id string) (domain.Deployment, error)
	// ListDeploymentsByAppID returns deployments for one app.
	ListDeploymentsByAppID(ctx context.Context, appID string) ([]domain.Deployment, error)

	// TakeNextQueuedDeployment returns and removes the next queued deployment.
	TakeNextQueuedDeployment(ctx context.Context) (domain.Deployment, error)
	// UpdateDeployment updates an existing deployment record.
	UpdateDeployment(ctx context.Context, deployment domain.Deployment) error
}
