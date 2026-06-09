// Package service wires control-plane business services.
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spacescale/core/control/db/sqlc"
)

var (
	// ErrBootstrapRejected reports that a bootstrap token did not match a provisioning node.
	ErrBootstrapRejected = errors.New("node bootstrap rejected")
)

// BootstrapInput contains the host facts sent during node bootstrap.
type BootstrapInput struct {
	Token       string
	TotalCores  int32
	TotalRAMMb  int64
	TotalDiskMb int64
}

// BootstrapResult contains the durable node identity assigned at bootstrap.
type BootstrapResult struct {
	NodeID string
	Region string
}

// BootstrapService registers provisioning nodes that present a valid bootstrap token.
type BootstrapService struct {
	queries *sqlc.Queries
	pool    *pgxpool.Pool
}

// NewBootstrapService constructs the fleet bootstrap service.
func NewBootstrapService(queries *sqlc.Queries, pool *pgxpool.Pool) *BootstrapService {
	return &BootstrapService{queries: queries, pool: pool}
}

// Register records bootstrap facts for a provisioning node and returns its assigned identity.
func (s *BootstrapService) Register(ctx context.Context, input BootstrapInput) (BootstrapResult, error) {
	tokenHash := hashBootstrapToken(input.Token)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return BootstrapResult{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	qtx := s.queries.WithTx(tx)
	node, err := qtx.UpdateProvisioningNodeFromBootstrap(ctx, sqlc.UpdateProvisioningNodeFromBootstrapParams{
		TotalCores:         input.TotalCores,
		TotalRamMb:         input.TotalRAMMb,
		TotalDiskMb:        input.TotalDiskMb,
		BootstrapTokenHash: new(hashBootstrapToken(input.Token)),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BootstrapResult{}, ErrBootstrapRejected
		}
		return BootstrapResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BootstrapResult{}, err
	}
	return BootstrapResult{
		NodeID: node.ID.String(),
		Region: node.Region,
	}, nil
}

func hashBootstrapToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
