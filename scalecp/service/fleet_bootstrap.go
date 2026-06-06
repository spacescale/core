package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spacescale/core/scalecp/db/sqlc"
)

var (
	ErrInvalidBootstrapRequest = errors.New("invalid node bootstrap request")
	ErrBootstrapRejected       = errors.New("node bootstrap rejected")
)

type BootstrapInput struct {
	Token       string
	TotalCores  int32
	TotalRamMb  int64
	TotalDiskMb int64
}

type BootstrapResult struct {
	NodeID string
	Region string
}

type BootstrapService struct {
	queries *sqlc.Queries
	pool    *pgxpool.Pool
}

func NewBootstrapService(queries *sqlc.Queries, pool *pgxpool.Pool) *BootstrapService {
	return &BootstrapService{queries: queries, pool: pool}
}

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
		TotalRamMb:         input.TotalRamMb,
		TotalDiskMb:        input.TotalDiskMb,
		BootstrapTokenHash: &tokenHash,
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
