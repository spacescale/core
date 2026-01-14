package contracts

import (
	"context"

	"github.com/t0gun/paas/internal/domain"
)

type Runtime interface {
	Deploy(ctx context.Context, app domain.App) (url string, err error)
}
