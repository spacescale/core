package fake

import (
	"context"
	"fmt"
	"strings"

	"github.com/t0gun/paas/internal/domain"
)

type Runtime struct {
	BaseDomain string
	Scheme     string // https or http
}

func New(baseDomain string) *Runtime {
	return &Runtime{
		BaseDomain: strings.TrimSpace(baseDomain),
		Scheme:     "https",
	}
}

func (r *Runtime) Deploy(ctx context.Context, app domain.App) (string, error) {
	return fmt.Sprintf("%s://%s.%s", r.Scheme, app.Name, r.BaseDomain), nil
}
