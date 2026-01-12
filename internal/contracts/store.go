package contracts

import (
	"context"

	"github.com/t0gun/paas/internal/domain"
)

type Store interface {
	CreateApp(ctx context.Context, app domain.App) error
	GetAppByID(ctx context.Context, id string) (domain.App, error)
	GetAppByName(ctx context.Context, name string) (domain.App, error)
	ListApps(ctx context.Context) ([]domain.App, error)

	CreateDeployment(ctx context.Context, dep domain.Deployment) error
	GetDeploymentByID(ctx context.Context, id string) (domain.Deployment, error)
	ListDeploymentsByAppID(ctx context.Context, appID string) ([]domain.Deployment, error)
}
