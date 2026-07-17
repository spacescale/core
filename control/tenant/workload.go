// Package tenant implements authenticated user, workspace, project, and workload workflows.
//
// This file implements workload-creation workflows for authenticated project owners.
//
// Responsibilities in this file:
//   - Validate and normalize create-workload input (name/image/port/visibility/env vars).
//   - Enforce ownership boundaries for workspace, project, and optional registry
//     credential attachment.
//   - Persist workload + initial deployment + related rows atomically so externally
//     observable lifecycle state is never partially written.
//   - Encrypt all env var values before persistence.
package tenant

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spacescale/core/control/db/sqlc"
	"github.com/spacescale/core/shared/pb/v1"
	"github.com/spacescale/core/shared/secret"
)

const (
	defaultWorkloadRuntimePort = 8080 // fallback when create input omits runtime port.
	defaultRootDiskMB          = 5120 // persisted legacy metadata; not sent to scaled launch shape.
)

const microvmResourceTypeDeployment = "deployment"

var errWorkloadSlugConflict = errors.New("workload slug conflict")

// Workload is the service-layer model returned to HTTP handlers.
type Workload struct {
	ID             string    // workload UUID as string.
	ProjectID      string    // owning project UUID as string.
	Name           string    // user-facing workload name.
	Slug           string    // URL-safe workload identifier.
	Subdomain      string    // DNS label used for routing.
	ImageRef       string    // OCI image reference used for deployment.
	TargetReplicas int32     // desired replica count for the active rollout.
	PrimaryRegion  string    // requested home region.
	RuntimePort    int32     // container port exposed by the workload.
	Status         string    // workload lifecycle state.
	IsPublic       bool      // whether public ingress is enabled.
	CreatedAt      time.Time // record creation timestamp.
	UpdatedAt      time.Time // record last-update timestamp.
}

// CreateWorkloadResult contains the persisted workload and initial dispatch request fields.
type CreateWorkloadResult struct {
	Workload     Workload
	WorkloadID   uuid.UUID
	DeploymentID uuid.UUID
	MicroVMID    uuid.UUID
	Shape        *pb.MicroVMShape
	Env          map[string]string
}

// DispatchAssignment records which node accepted a microVM placement.
type DispatchAssignment struct {
	WorkloadID   uuid.UUID
	DeploymentID uuid.UUID
	MicroVMID    uuid.UUID
	NodeID       uuid.UUID
}

// DispatchFailure records launch failure details for workload deployment state.
type DispatchFailure struct {
	WorkloadID   uuid.UUID
	DeploymentID uuid.UUID
	MicroVMID    uuid.UUID
	Reason       string
}

// WorkloadComputeInput is the explicit compute shape requested by the caller.
type WorkloadComputeInput struct {
	VCPU      uint32
	MemoryMB  uint64
	Dedicated bool
}

// WorkloadEnvVarInput defines one env var in create requests.
type WorkloadEnvVarInput struct {
	Key      string // variable name, normalized to uppercase.
	Value    string // raw variable value before storage encryption.
	IsSecret bool   // whether callers should treat this variable as sensitive.
}

// CreateWorkloadParams contains create-workload input from handlers.
type CreateWorkloadParams struct {
	Name                 string                // optional; derived from ImageRef when empty.
	ImageRef             string                // required OCI image reference.
	Compute              WorkloadComputeInput  // required to be resolved compute request.
	PrimaryRegion        string                // placement region; empty means auto-placement at the API layer.
	RuntimePort          *int                  // optional; nil uses defaultWorkloadRuntimePort.
	IsPublic             *bool                 // optional; nil defaults to false.
	RegistryCredentialID string                // optional; must belong to same project.
	EnvVars              []WorkloadEnvVarInput // optional; validated for format and duplicates.
}

// WorkloadService owns workload creation workflows.
type WorkloadService struct {
	queries   *sqlc.Queries // SQLC-backed persistence operations.
	pool      *pgxpool.Pool // transaction entrypoint for atomic create-workload writes.
	envCipher *secret.Box   // encrypts stored workload env vars.
}

// NewWorkloadService constructs an WorkloadService with shared query and crypto deps.
func NewWorkloadService(queries *sqlc.Queries, pool *pgxpool.Pool, envCipher *secret.Box) *WorkloadService {
	return &WorkloadService{queries: queries, pool: pool, envCipher: envCipher}
}

// CreateWorkload creates a workload under an owned workspace/project and applies defaults.
// If Name is empty, it is derived from image repository name.
//
// Atomic write contract:
// - workload row (status=queued)
// - initial queued deployment row
// - optional workload<->registry association
// - optional env var rows (encrypted values)
func (s *WorkloadService) CreateWorkload(ctx context.Context, ownerUserID, workspaceID, projectID string, params CreateWorkloadParams) (CreateWorkloadResult, error) {
	workspaceUUID, projectUUID, err := s.authorizeOwnedProject(ctx, ownerUserID, workspaceID, projectID)
	if err != nil {
		return CreateWorkloadResult{}, err
	}

	shape, ok := normalizeWorkloadCompute(params.Compute)
	if !ok {
		return CreateWorkloadResult{}, ErrInvalidInput
	}

	primaryRegion := strings.ToLower(strings.TrimSpace(params.PrimaryRegion))
	name, ok := normalizeOrDeriveWorkloadName(params.Name, params.ImageRef)
	if !ok {
		return CreateWorkloadResult{}, ErrInvalidInput
	}
	runtimePort := defaultWorkloadRuntimePort
	if params.RuntimePort != nil {
		runtimePort = *params.RuntimePort
	}
	isPublic := params.IsPublic != nil && *params.IsPublic
	envVars, ok := normalizeWorkloadEnvVars(params.EnvVars)
	if !ok {
		return CreateWorkloadResult{}, ErrInvalidInput
	}
	envMap := make(map[string]string, len(envVars))
	for _, env := range envVars {
		envMap[env.Key] = env.Value
	}
	registryCredentialID, hasRegistryCredential, err := s.resolveRegistryCredential(ctx, projectUUID, params.RegistryCredentialID)
	if err != nil {
		return CreateWorkloadResult{}, err
	}

	baseSlug := slugifyProjectName(name)
	if baseSlug == "" {
		return CreateWorkloadResult{}, ErrInvalidInput
	}

	// Retry only on slug/subdomain uniqueness conflicts. Each attempt must use a
	// fresh transaction because PostgreSQL aborts failed transactions.
	for i := range maxSlugRetries {
		slug := baseSlug
		if i > 0 {
			suffix, suffixErr := randomSuffix(suffixLength)
			if suffixErr != nil {
				return CreateWorkloadResult{}, suffixErr
			}
			slug = slugWithSuffix(baseSlug, suffix)
		}

		result, err := s.createWorkloadAttempt(ctx, createWorkloadAttemptParams{
			WorkspaceID:           workspaceUUID,
			ProjectID:             projectUUID,
			Name:                  name,
			Slug:                  slug,
			ImageRef:              params.ImageRef,
			PrimaryRegion:         primaryRegion,
			RuntimePort:           int32(runtimePort),
			IsPublic:              isPublic,
			Shape:                 shape,
			EnvVars:               envVars,
			EnvMap:                envMap,
			RegistryCredentialID:  registryCredentialID,
			HasRegistryCredential: hasRegistryCredential,
		})
		if errors.Is(err, errWorkloadSlugConflict) {
			continue
		}
		if err != nil {
			return CreateWorkloadResult{}, err
		}

		return result, nil
	}

	return CreateWorkloadResult{}, ErrConflict
}

// ListWorkloads returns workloads in a project the caller owns.
func (s *WorkloadService) ListWorkloads(ctx context.Context, ownerUserID, workspaceID, projectID string) ([]Workload, error) {
	_, projectUUID, err := s.authorizeOwnedProject(ctx, ownerUserID, workspaceID, projectID)
	if err != nil {
		return nil, err
	}

	rows, err := s.queries.ListWorkloadsByProjectID(ctx, projectUUID)
	if err != nil {
		return nil, err
	}

	out := make([]Workload, 0, len(rows))
	for _, row := range rows {
		out = append(out, workloadFromRow(row))
	}

	return out, nil
}

// GetWorkload returns one workload in a project the caller owns.
func (s *WorkloadService) GetWorkload(ctx context.Context, ownerUserID, workspaceID, projectID, workloadID string) (Workload, error) {
	_, projectUUID, err := s.authorizeOwnedProject(ctx, ownerUserID, workspaceID, projectID)
	if err != nil {
		return Workload{}, err
	}

	appUUID, err := uuid.Parse(strings.TrimSpace(workloadID))
	if err != nil {
		return Workload{}, ErrInvalidInput
	}

	row, err := s.queries.GetWorkloadByIDAndProjectID(ctx, sqlc.GetWorkloadByIDAndProjectIDParams{
		ID:        appUUID,
		ProjectID: projectUUID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Workload{}, ErrUnauthorized
		}

		return Workload{}, err
	}

	return workloadFromRow(row), nil
}

// MarkDeploying records placement and marks workload deployment in progress.
func (s *WorkloadService) MarkDeploying(ctx context.Context, params DispatchAssignment) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	txQueries := s.queries.WithTx(tx)

	if _, err := txQueries.MarkDeploymentDeploying(ctx, params.DeploymentID); err != nil {
		return err
	}

	if _, err := txQueries.MarkWorkloadDeploying(ctx, params.WorkloadID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// MarkMicroVMStarting marks a microVM accepted by a node as starting.
func (s *WorkloadService) MarkMicroVMStarting(ctx context.Context, microVMID uuid.UUID) error {
	_, err := s.queries.MarkMicroVMStarting(ctx, microVMID)

	return err
}

// MarkDispatchFailed marks workload, deployment, and microVM records failed after dispatch failure.
func (s *WorkloadService) MarkDispatchFailed(ctx context.Context, params DispatchFailure) error {
	errMsg := params.Reason
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	txQueries := s.queries.WithTx(tx)

	if _, err := txQueries.MarkMicroVMFailed(ctx, sqlc.MarkMicroVMFailedParams{
		ID:           params.MicroVMID,
		ErrorMessage: &errMsg,
	}); err != nil {
		return err
	}

	if _, err := txQueries.MarkDeploymentFailed(ctx, sqlc.MarkDeploymentFailedParams{
		ID:           params.DeploymentID,
		ErrorMessage: &errMsg,
	}); err != nil {
		return err
	}

	if _, err := txQueries.MarkWorkloadFailed(ctx, params.WorkloadID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

type createWorkloadAttemptParams struct {
	WorkspaceID           uuid.UUID
	ProjectID             uuid.UUID
	Name                  string
	Slug                  string
	ImageRef              string
	PrimaryRegion         string
	RuntimePort           int32
	IsPublic              bool
	Shape                 *pb.MicroVMShape
	EnvVars               []WorkloadEnvVarInput
	EnvMap                map[string]string
	RegistryCredentialID  uuid.UUID
	HasRegistryCredential bool
}

func (s *WorkloadService) createWorkloadAttempt(ctx context.Context, params createWorkloadAttemptParams) (CreateWorkloadResult, error) {
	vcpu, ramMB, volumeMB, ok := microVMShapeDBValues(params.Shape)
	if !ok {
		return CreateWorkloadResult{}, ErrInvalidInput
	}
	if params.RuntimePort < 0 {
		return CreateWorkloadResult{}, ErrInvalidInput
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return CreateWorkloadResult{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	txQueries := s.queries.WithTx(tx)

	row, err := txQueries.CreateWorkload(ctx, sqlc.CreateWorkloadParams{
		ProjectID:     params.ProjectID,
		Name:          params.Name,
		Slug:          params.Slug,
		Subdomain:     params.Slug,
		ImageRef:      params.ImageRef,
		PrimaryRegion: params.PrimaryRegion,
		RuntimePort:   params.RuntimePort,
		IsPublic:      params.IsPublic,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return CreateWorkloadResult{}, errWorkloadSlugConflict
		}

		return CreateWorkloadResult{}, err
	}

	deployment, err := txQueries.CreateQueuedDeployment(ctx, sqlc.CreateQueuedDeploymentParams{
		WorkloadID:  row.ID,
		ImageRef:    params.ImageRef,
		RuntimePort: params.RuntimePort,
	})
	if err != nil {
		return CreateWorkloadResult{}, err
	}

	// The deployment owns the microvm so one workload can have many deployments and
	// each deployment can have many replicas without mixing rollout generations.
	microvm, err := txQueries.CreateQueuedMicroVM(ctx, sqlc.CreateQueuedMicroVMParams{
		WorkspaceID:  params.WorkspaceID,
		ResourceType: microvmResourceTypeDeployment,
		ResourceID:   &deployment.ID,
		Region:       row.PrimaryRegion,
		Vcpu:         vcpu,
		RamMb:        ramMB,
		CpuMode:      cpuModeString(params.Shape.GetCpuMode()),
		RootDiskMb:   defaultRootDiskMB,
		VolumeMb:     volumeMB,
	})
	if err != nil {
		return CreateWorkloadResult{}, err
	}

	if params.HasRegistryCredential {
		if err := txQueries.UpsertWorkloadRegistryCredential(ctx, sqlc.UpsertWorkloadRegistryCredentialParams{
			WorkloadID:           row.ID,
			RegistryCredentialID: params.RegistryCredentialID,
		}); err != nil {
			return CreateWorkloadResult{}, err
		}
	}

	if err := s.createWorkloadEnvVars(ctx, txQueries, row.ID, params.EnvVars); err != nil {
		return CreateWorkloadResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return CreateWorkloadResult{}, err
	}

	return CreateWorkloadResult{
		Workload:     workloadFromRow(row),
		WorkloadID:   row.ID,
		DeploymentID: deployment.ID,
		MicroVMID:    microvm.ID,
		Shape:        cloneMicroVMShape(params.Shape),
		Env:          params.EnvMap,
	}, nil
}

func (s *WorkloadService) createWorkloadEnvVars(ctx context.Context, queries *sqlc.Queries, workloadID uuid.UUID, envVars []WorkloadEnvVarInput) error {
	bulkEnvVars := make([]sqlc.CreateWorkloadEnvVarsParams, 0, len(envVars))
	for _, env := range envVars {
		valueEncrypted, err := s.envCipher.Encrypt(env.Value)
		if err != nil {
			return err
		}
		bulkEnvVars = append(bulkEnvVars, sqlc.CreateWorkloadEnvVarsParams{
			WorkloadID:     workloadID,
			Key:            env.Key,
			ValueEncrypted: valueEncrypted,
			IsSecret:       env.IsSecret,
		})
	}

	if len(bulkEnvVars) == 0 {
		return nil
	}
	if _, err := queries.CreateWorkloadEnvVars(ctx, bulkEnvVars); err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}

		return err
	}

	return nil
}

// authorizeOwnedProject verifies that the owner can access the workspace and
// project, and that the project belongs to that workspace.
func (s *WorkloadService) authorizeOwnedProject(ctx context.Context, ownerUserID, workspaceID, projectID string) (uuid.UUID, uuid.UUID, error) {
	ownerUUID, err := uuid.Parse(strings.TrimSpace(ownerUserID))
	if err != nil {
		return uuid.Nil, uuid.Nil, ErrInvalidInput
	}
	workspaceUUID, err := uuid.Parse(strings.TrimSpace(workspaceID))
	if err != nil {
		return uuid.Nil, uuid.Nil, ErrInvalidInput
	}
	projectUUID, err := uuid.Parse(strings.TrimSpace(projectID))
	if err != nil {
		return uuid.Nil, uuid.Nil, ErrInvalidInput
	}

	_, err = s.queries.CheckProjectOwnership(ctx, sqlc.CheckProjectOwnershipParams{
		ProjectID:   projectUUID,
		WorkspaceID: workspaceUUID,
		OwnerUserID: ownerUUID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, uuid.Nil, ErrUnauthorized
		}

		return uuid.Nil, uuid.Nil, err
	}

	return workspaceUUID, projectUUID, nil
}

// resolveRegistryCredential validates an optional registry credential id and
// enforces project ownership for that credential.
func (s *WorkloadService) resolveRegistryCredential(ctx context.Context, projectID uuid.UUID, rawCredentialID string) (uuid.UUID, bool, error) {
	if rawCredentialID == "" {
		return uuid.Nil, false, nil
	}

	credentialID, err := uuid.Parse(strings.TrimSpace(rawCredentialID))
	if err != nil {
		return uuid.Nil, false, ErrInvalidInput
	}

	_, err = s.queries.GetRegistryCredentialByIDAndProjectID(ctx, sqlc.GetRegistryCredentialByIDAndProjectIDParams{
		ID:        credentialID,
		ProjectID: projectID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, false, ErrUnauthorized
		}

		return uuid.Nil, false, err
	}

	return credentialID, true, nil
}

func normalizeWorkloadCompute(raw WorkloadComputeInput) (*pb.MicroVMShape, bool) {
	cpuMode := pb.CpuMode_CPU_MODE_SHARED
	if raw.Dedicated {
		cpuMode = pb.CpuMode_CPU_MODE_PINNED
	}

	return &pb.MicroVMShape{
		Vcpu:     raw.VCPU,
		RamMb:    raw.MemoryMB,
		CpuMode:  cpuMode,
		VolumeMb: 0,
	}, true
}

// normalizeOrDeriveWorkloadName returns a validated workload name.
func normalizeOrDeriveWorkloadName(rawName, imageRef string) (string, bool) {
	name := strings.TrimSpace(rawName)
	if name == "" {
		derived, ok := deriveWorkloadNameFromImageRef(imageRef)
		if !ok {
			return "", false
		}
		name = derived
	}

	return name, name != ""
}

// deriveWorkloadNameFromImageRef extracts a display name candidate from an image
// reference by dropping optional digest and tag components.
func deriveWorkloadNameFromImageRef(imageRef string) (string, bool) {
	withoutDigest := imageRef
	if at := strings.IndexByte(withoutDigest, '@'); at >= 0 {
		withoutDigest = withoutDigest[:at]
	}

	lastSegment := withoutDigest
	if slash := strings.LastIndex(withoutDigest, "/"); slash >= 0 {
		if slash+1 >= len(withoutDigest) {
			return "", false
		}
		lastSegment = withoutDigest[slash+1:]
	}

	if colon := strings.LastIndex(lastSegment, ":"); colon >= 0 {
		lastSegment = lastSegment[:colon]
	}

	return lastSegment, lastSegment != ""
}

// normalizeIsPublic resolves optional public exposure input.
// normalizeWorkloadEnvVars validates env var entries, rejects duplicate keys, and
// returns a normalized copy ready for persistence.
func normalizeWorkloadEnvVars(raw []WorkloadEnvVarInput) ([]WorkloadEnvVarInput, bool) {
	if len(raw) == 0 {
		return nil, true
	}

	out := make([]WorkloadEnvVarInput, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, item := range raw {
		key := strings.TrimSpace(item.Key)
		if _, exists := seen[key]; exists {
			return nil, false
		}
		seen[key] = struct{}{}
		out = append(out, WorkloadEnvVarInput{
			Key:      key,
			Value:    item.Value,
			IsSecret: item.IsSecret,
		})
	}

	return out, true
}

// workloadFromRow maps a SQLC workload row into the service Workload model.
func workloadFromRow(row sqlc.Workload) Workload {
	return Workload{
		ID:             row.ID.String(),
		ProjectID:      row.ProjectID.String(),
		Name:           row.Name,
		Slug:           row.Slug,
		Subdomain:      row.Subdomain,
		ImageRef:       row.ImageRef,
		TargetReplicas: row.TargetReplicas,
		PrimaryRegion:  row.PrimaryRegion,
		RuntimePort:    row.RuntimePort,
		Status:         row.Status,
		IsPublic:       row.IsPublic,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
}

func cpuModeString(mode pb.CpuMode) string {
	if mode == pb.CpuMode_CPU_MODE_PINNED {
		return "pinned"
	}

	return "shared"
}

func microVMShapeDBValues(shape *pb.MicroVMShape) (int32, int64, int64, bool) {
	if shape == nil || shape.GetVcpu() > math.MaxInt32 || shape.GetRamMb() > math.MaxInt64 || shape.GetVolumeMb() > math.MaxInt64 {
		return 0, 0, 0, false
	}

	return int32(shape.GetVcpu()), int64(shape.GetRamMb()), int64(shape.GetVolumeMb()), true //nolint:gosec // Bounds checked above before DB-width casts.
}

func cloneMicroVMShape(shape *pb.MicroVMShape) *pb.MicroVMShape {
	if shape == nil {
		return nil
	}

	return &pb.MicroVMShape{
		Vcpu:     shape.GetVcpu(),
		RamMb:    shape.GetRamMb(),
		CpuMode:  shape.GetCpuMode(),
		VolumeMb: shape.GetVolumeMb(),
	}
}
