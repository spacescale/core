// This file implements app-creation workflows for authenticated project owners.
//
// Responsibilities in this file:
// - Validate and normalize create-app input (name/image/port/visibility/env vars).
// - Enforce ownership boundaries for workspace, project, and optional registry
//   credential attachment.
// - Persist app + initial deployment + related rows atomically so externally
//   observable lifecycle state is never partially written.
// - Encrypt all env var values before persistence.
//
// Lifecycle invariant:
// - An app row with status=queued is persisted only when its initial queued
//   deployment row is persisted in the same transaction.

package tenant

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spacescale/core/internal/scalecp/db/sqlc"
)

const (
	appPrimaryRegionMaxLen = 32
	defaultAppRuntimePort  = 8080 // fallback when create input omits runtime port.
	appNameMaxChars        = 63   // maximum app display-name length.
	appImageRefMaxChars    = 1024 // maximum accepted image reference length.
	appEnvVarKeyMaxChars   = 128  // maximum environment-variable key length.
	appEnvVarValueMaxRunes = 8192 // maximum environment-variable value length in runes.
	appEnvVarsMaxCount     = 50   // maximum environment variables accepted per create request.
)

// App is the service-layer model returned to HTTP handlers.
type App struct {
	ID            string    // app UUID as string.
	ProjectID     string    // owning project UUID as string.
	Name          string    // user-facing app name.
	Slug          string    // URL-safe app identifier.
	Subdomain     string    // DNS label used for routing.
	ImageRef      string    // OCI image reference used for deployment.
	Tier          string    // requested compute tier.
	PrimaryRegion string    // requested home region.
	RuntimePort   int32     // container port exposed by the app.
	Status        string    // app lifecycle state.
	IsPublic      bool      // whether public ingress is enabled.
	CreatedAt     time.Time // record creation timestamp.
	UpdatedAt     time.Time // record last-update timestamp.
}

// AppEnvVarInput defines one env var in create requests.
type AppEnvVarInput struct {
	Key      string // variable name, normalized to uppercase.
	Value    string // raw variable value before storage encryption.
	IsSecret bool   // whether callers should treat this variable as sensitive.
}

// CreateAppParams contains create-app input from handlers.
type CreateAppParams struct {
	Name                 string           // optional; derived from ImageRef when empty.
	ImageRef             string           // required OCI image reference.
	Tier                 string           // required compute tier.
	PrimaryRegion        string           // required placement region.
	RuntimePort          *int             // optional; nil uses defaultAppRuntimePort.
	IsPublic             *bool            // optional; nil defaults to false.
	RegistryCredentialID string           // optional; must belong to same project.
	EnvVars              []AppEnvVarInput // optional; validated for format and duplicates.
}

// AppService owns app creation workflows.
type AppService struct {
	queries   *sqlc.Queries   // SQLC-backed persistence operations.
	pool      *pgxpool.Pool   // transaction entrypoint for atomic create-app writes.
	envCipher *EnvValueCipher // encrypts and decrypts stored app env vars.
}

// NewAppService constructs an AppService with shared query and crypto deps.
func NewAppService(queries *sqlc.Queries, pool *pgxpool.Pool, envCipher *EnvValueCipher) *AppService {
	return &AppService{queries: queries, pool: pool, envCipher: envCipher}
}

// CreateApp creates an app under an owned workspace/project and applies defaults.
// If Name is empty, it is derived from image repository name.
//
// Atomic write contract:
// - app row (status=queued)
// - initial queued deployment row
// - optional app<->registry association
// - optional env var rows (encrypted values)
func (s *AppService) CreateApp(ctx context.Context, ownerUserID, workspaceID, projectID string, p CreateAppParams) (App, error) {
	projectUUID, err := s.authorizeOwnedProject(ctx, ownerUserID, workspaceID, projectID)
	if err != nil {
		return App{}, err
	}

	imageRef, ok := normalizeImageRef(p.ImageRef)
	if !ok {
		return App{}, ErrInvalidInput
	}
	tier, ok := normalizeAppTier(p.Tier)
	if !ok {
		return App{}, ErrInvalidInput
	}
	primaryRegion, ok := normalizeAppPrimaryRegion(p.PrimaryRegion)
	if !ok {
		return App{}, ErrInvalidInput
	}
	name, ok := normalizeOrDeriveAppName(p.Name, imageRef)
	if !ok {
		return App{}, ErrInvalidInput
	}
	runtimePort, ok := normalizeRuntimePort(p.RuntimePort)
	if !ok {
		return App{}, ErrInvalidInput
	}
	isPublic := normalizeIsPublic(p.IsPublic)
	envVars, ok := normalizeEnvVars(p.EnvVars)
	if !ok {
		return App{}, ErrInvalidInput
	}
	registryCredentialID, hasRegistryCredential, err := s.resolveRegistryCredential(ctx, projectUUID, p.RegistryCredentialID)
	if err != nil {
		return App{}, err
	}

	baseSlug := slugifyProjectName(name)
	if baseSlug == "" {
		return App{}, ErrInvalidInput
	}

	// Retry only on slug/subdomain uniqueness conflicts. Each attempt must use a
	// fresh transaction because PostgreSQL aborts failed transactions.
	for i := 0; i < maxSlugRetries; i++ {
		slug := baseSlug
		if i > 0 {
			suffix, suffixErr := randomSuffix(suffixLength)
			if suffixErr != nil {
				return App{}, suffixErr
			}
			slug = slugWithSuffix(baseSlug, suffix)
		}

		tx, beginErr := s.pool.Begin(ctx)
		if beginErr != nil {
			return App{}, beginErr
		}
		txQueries := s.queries.WithTx(tx)

		row, createErr := txQueries.CreateApp(ctx, sqlc.CreateAppParams{
			ProjectID:     projectUUID,
			Name:          name,
			Slug:          slug,
			Subdomain:     slug,
			ImageRef:      imageRef,
			Tier:          tier,
			PrimaryRegion: primaryRegion,
			RuntimePort:   runtimePort,
			IsPublic:      isPublic,
		})
		if createErr != nil {
			_ = tx.Rollback(ctx)
			if isUniqueViolation(createErr) {
				continue
			}
			return App{}, createErr
		}

		deployment, err := txQueries.CreateQueuedDeployment(ctx, sqlc.CreateQueuedDeploymentParams{
			AppID:       row.ID,
			ImageRef:    imageRef,
			RuntimePort: runtimePort,
		})
		if err != nil {
			_ = tx.Rollback(ctx)
			return App{}, err
		}

		if _, err := txQueries.CreateQueuedMachine(ctx, sqlc.CreateQueuedMachineParams{
			AppID:        row.ID,
			DeploymentID: deployment.ID,
			Region:       row.PrimaryRegion,
			Tier:         row.Tier,
		}); err != nil {
			_ = tx.Rollback(ctx)
			return App{}, err
		}

		if hasRegistryCredential {
			if err := txQueries.UpsertAppRegistryCredential(ctx, sqlc.UpsertAppRegistryCredentialParams{
				AppID:                row.ID,
				RegistryCredentialID: registryCredentialID,
			}); err != nil {
				_ = tx.Rollback(ctx)
				return App{}, err
			}
		}

		bulkEnvVars := make([]sqlc.CreateAppEnvVarsParams, 0, len(envVars))
		for _, env := range envVars {
			valueEncrypted, encErr := s.envCipher.EncryptForStorage(env.Value, EnvValueRowContext{
				AppID: row.ID,
				Key:   env.Key,
			})
			if encErr != nil {
				_ = tx.Rollback(ctx)
				return App{}, encErr
			}
			bulkEnvVars = append(bulkEnvVars, sqlc.CreateAppEnvVarsParams{
				AppID:          row.ID,
				Key:            env.Key,
				ValueEncrypted: valueEncrypted,
				IsSecret:       env.IsSecret,
			})
		}

		if len(bulkEnvVars) > 0 {
			if _, err := txQueries.CreateAppEnvVars(ctx, bulkEnvVars); err != nil {
				_ = tx.Rollback(ctx)
				if isUniqueViolation(err) {
					return App{}, ErrConflict
				}
				return App{}, err
			}
		}

		if err := tx.Commit(ctx); err != nil {
			return App{}, err
		}

		return appFromRow(row), nil
	}

	return App{}, ErrConflict
}

// authorizeOwnedProject verifies that the owner can access the workspace and
// project, and that the project belongs to that workspace.
func (s *AppService) authorizeOwnedProject(ctx context.Context, ownerUserID, workspaceID, projectID string) (uuid.UUID, error) {
	ownerUUID, ok := parseUUID(ownerUserID)
	if !ok {
		return uuid.Nil, ErrInvalidInput
	}
	workspaceUUID, ok := parseUUID(workspaceID)
	if !ok {
		return uuid.Nil, ErrInvalidInput
	}
	projectUUID, ok := parseUUID(projectID)
	if !ok {
		return uuid.Nil, ErrInvalidInput
	}

	_, err := s.queries.CheckProjectOwnership(ctx, sqlc.CheckProjectOwnershipParams{
		ProjectID:   projectUUID,
		WorkspaceID: workspaceUUID,
		OwnerUserID: ownerUUID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrUnauthorized
		}
		return uuid.Nil, err
	}

	return projectUUID, nil
}

// resolveRegistryCredential validates an optional registry credential id and
// enforces project ownership for that credential.
func (s *AppService) resolveRegistryCredential(ctx context.Context, projectID uuid.UUID, rawCredentialID string) (uuid.UUID, bool, error) {
	if rawCredentialID == "" {
		return uuid.Nil, false, nil
	}

	credentialID, ok := parseUUID(rawCredentialID)
	if !ok {
		return uuid.Nil, false, ErrInvalidInput
	}

	_, err := s.queries.GetRegistryCredentialByIDAndProjectID(ctx, sqlc.GetRegistryCredentialByIDAndProjectIDParams{
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

// normalizeImageRef trims and validates image reference input.
func normalizeImageRef(raw string) (string, bool) {
	ref := strings.TrimSpace(raw)
	if ref == "" || len(ref) > appImageRefMaxChars {
		return "", false
	}
	if strings.ContainsAny(ref, " \t\r\n") {
		return "", false
	}
	return ref, true
}

func normalizeAppTier(raw string) (string, bool) {
	tier := strings.ToLower(strings.TrimSpace(raw))
	switch tier {
	case "starter", "growth", "scale":
		return tier, true
	default:
		return "", false
	}
}

func normalizeAppPrimaryRegion(raw string) (string, bool) {
	region := strings.ToLower(strings.TrimSpace(raw))
	if region == "" || len(region) > appPrimaryRegionMaxLen {
		return "", false
	}
	if region[0] == '-' || region[len(region)-1] == '-' {
		return "", false
	}
	for i := 0; i < len(region); i++ {
		ch := region[i]
		if ch == '-' {
			continue
		}
		if !isASCIIAlphaNum(ch) {
			return "", false
		}
	}
	return region, true
}

// normalizeOrDeriveAppName returns a validated app name.
func normalizeOrDeriveAppName(rawName, imageRef string) (string, bool) {
	name := strings.TrimSpace(rawName)
	if name == "" {
		derived, ok := deriveAppNameFromImageRef(imageRef)
		if !ok {
			return "", false
		}
		name = derived
	}
	if utf8.RuneCountInString(name) > appNameMaxChars {
		return "", false
	}
	return name, name != ""
}

// deriveAppNameFromImageRef extracts a display name candidate from an image
// reference by dropping optional digest and tag components.
func deriveAppNameFromImageRef(imageRef string) (string, bool) {
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

// normalizeRuntimePort returns a validated runtime port.
func normalizeRuntimePort(raw *int) (int32, bool) {
	port := defaultAppRuntimePort
	if raw != nil {
		port = *raw
	}
	if port < 1 || port > 65535 {
		return 0, false
	}
	return int32(port), true
}

// normalizeIsPublic resolves optional public exposure input.
func normalizeIsPublic(raw *bool) bool {
	if raw == nil {
		return false
	}
	return *raw
}

// normalizeEnvVars validates env var entries, rejects duplicate keys, and
// returns a normalized copy ready for persistence.
func normalizeEnvVars(raw []AppEnvVarInput) ([]AppEnvVarInput, bool) {
	if len(raw) == 0 {
		return nil, true
	}
	if len(raw) > appEnvVarsMaxCount {
		return nil, false
	}

	out := make([]AppEnvVarInput, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, item := range raw {
		key, ok := normalizeEnvVarKey(item.Key)
		if !ok {
			return nil, false
		}
		if utf8.RuneCountInString(item.Value) > appEnvVarValueMaxRunes {
			return nil, false
		}
		if _, exists := seen[key]; exists {
			return nil, false
		}
		seen[key] = struct{}{}
		out = append(out, AppEnvVarInput{
			Key:      key,
			Value:    item.Value,
			IsSecret: item.IsSecret,
		})
	}

	return out, true
}

// normalizeEnvVarKey validates shell-style env var keys and normalizes to
// uppercase. The first character must be an uppercase letter or underscore.
func normalizeEnvVarKey(raw string) (string, bool) {
	key := strings.ToUpper(strings.TrimSpace(raw))
	if key == "" || len(key) > appEnvVarKeyMaxChars {
		return "", false
	}
	if !(isASCIIUpper(key[0]) || key[0] == '_') {
		return "", false
	}
	for i := 1; i < len(key); i++ {
		ch := key[i]
		if isASCIIUpper(ch) || isASCIIDigit(ch) || ch == '_' {
			continue
		}
		return "", false
	}
	return key, true
}

func isASCIIUpper(ch byte) bool {
	return ch >= 'A' && ch <= 'Z'
}

func isASCIIDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

// appFromRow maps a SQLC app row into the service App model.
func appFromRow(r sqlc.App) App {
	return App{
		ID:            r.ID.String(),
		ProjectID:     r.ProjectID.String(),
		Name:          r.Name,
		Slug:          r.Slug,
		Subdomain:     r.Subdomain,
		ImageRef:      r.ImageRef,
		Tier:          r.Tier,
		PrimaryRegion: r.PrimaryRegion,
		RuntimePort:   r.RuntimePort,
		Status:        r.Status,
		IsPublic:      r.IsPublic,
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
	}
}
