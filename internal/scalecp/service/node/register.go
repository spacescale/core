package node

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spacescale/core/internal/scalecp/db/sqlc"
	scalepb "github.com/spacescale/core/internal/shared/pb/v1"
)

var (
	maxInt32Uint32 = ^uint32(0) >> 1 // Max value for signed 32-bit int. Used as a 'No Limit' flag for CPU thread quotas.
	maxInt64Uint64 = ^uint64(0) >> 1 // Max value for signed 64-bit int. Used as a 'No Limit' flag for RAM/Disk MB allocations.

	ErrInvalidBootstrapRequest = errors.New("invalid node bootstrap request")
	ErrBootstrapRejected       = errors.New("node bootstrap rejected")
)

type RegisterResult struct {
	NodeID string
	Region string
}

type Registrar struct {
	queries *sqlc.Queries
	pool    *pgxpool.Pool
}

func NewRegistrar(queries *sqlc.Queries, pool *pgxpool.Pool) *Registrar {
	if queries == nil {
		panic("node.NewRegistrar requires non-nil queries")
	}
	if pool == nil {
		panic("node.NewRegistrar requires non-nil db pool")
	}
	return &Registrar{queries: queries, pool: pool}
}

// Register handles the "Secret Zero" bootstrap process for a new node.
// It validates the hardware's physical truth (CPU, RAM, Disk) reported by the
// scaled daemon against the one-time bootstrap token stored in the database.
//
// The process is strictly atomic:
//  1. It validates and hashes the provided bootstrap token.
//  2. It updates the 'metals' record, transitioning it from 'provisioning' to 'active'
//     while recording the discovered hardware specs.
//  3. It initializes the 'scaled' ledger entry, assigning a permanent NodeID.
//  4. It returns the assigned NodeID and Region to the daemon for future heartbeats.
//
// If the token hash does not match an existing 'provisioning' node, it returns
// ErrBootstrapRejected, effectively islanding the rogue hardware.
func (r *Registrar) Register(ctx context.Context, req *scalepb.NodeBootstrapRequest) (RegisterResult, error) {
	token, version, bootID, totalThreads, totalRamMB, totalDiskMB, err := validateBootstrapRequest(req)
	if err != nil {
		return RegisterResult{}, err
	}
	tokenHash := hashBootstrapToken(token)
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return RegisterResult{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	qtx := r.queries.WithTx(tx)
	metal, err := qtx.UpdateProvisioningMetalFromBootstrap(ctx, sqlc.UpdateProvisioningMetalFromBootstrapParams{
		TotalThreads:       totalThreads,
		TotalRamMb:         totalRamMB,
		TotalDiskMb:        totalDiskMB,
		BootstrapTokenHash: &tokenHash,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RegisterResult{}, ErrBootstrapRejected
		}
		return RegisterResult{}, err
	}
	scaled, err := qtx.UpsertScaledBootstrap(ctx, sqlc.UpsertScaledBootstrapParams{
		ID:      uuid.NewString(),
		Version: version,
		BootID:  bootID,
		MetalID: metal.ID,
	})
	if err != nil {
		return RegisterResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RegisterResult{}, err
	}
	return RegisterResult{
		NodeID: scaled.ID,
		Region: metal.Region,
	}, nil
}

func validateBootstrapRequest(req *scalepb.NodeBootstrapRequest) (string, string, string, int32, int64, int64, error) {
	if req == nil {
		return "", "", "", 0, 0, 0, ErrInvalidBootstrapRequest
	}
	token := strings.TrimSpace(req.GetBootstrapToken())
	if token == "" {
		return "", "", "", 0, 0, 0, ErrInvalidBootstrapRequest
	}
	version := strings.TrimSpace(req.GetVersion())
	if version == "" {
		return "", "", "", 0, 0, 0, ErrInvalidBootstrapRequest
	}
	bootID := strings.TrimSpace(req.GetBootId())
	if bootID == "" {
		return "", "", "", 0, 0, 0, ErrInvalidBootstrapRequest
	}
	totalThreads, ok := uint32ToInt32(req.GetTotalThreads())
	if !ok {
		return "", "", "", 0, 0, 0, ErrInvalidBootstrapRequest
	}
	totalRamMB, ok := uint64ToInt64(req.GetTotalRamMb())
	if !ok {
		return "", "", "", 0, 0, 0, ErrInvalidBootstrapRequest
	}
	totalDiskMB, ok := uint64ToInt64(req.GetTotalDiskMb())
	if !ok {
		return "", "", "", 0, 0, 0, ErrInvalidBootstrapRequest
	}
	return token, version, bootID, totalThreads, totalRamMB, totalDiskMB, nil
}

func hashBootstrapToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func uint32ToInt32(v uint32) (int32, bool) {
	if v == 0 || v > maxInt32Uint32 {
		return 0, false
	}
	return int32(v), true
}

func uint64ToInt64(v uint64) (int64, bool) {
	if v == 0 || v > maxInt64Uint64 {
		return 0, false
	}
	return int64(v), true
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
