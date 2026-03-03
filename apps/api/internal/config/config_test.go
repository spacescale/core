// This file tests startup configuration loading in package config.
//
// Why these tests exist:
// - loadAppConfig is the single entrypoint for startup config behavior.
// - Required-key validation must fail fast and predictably.
// - Cross-field normalization (for example DB pool bounds) should remain stable.
// - Security-sensitive constraints (hash mode requires secret) must not regress.
//
// Testing strategy:
// - Use environment-driven tests with t.Setenv so behavior matches real startup.
// - Keep tests focused on externally observable config outcomes.

package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// setBaselineEnv seeds required environment keys for successful config loading.
// Individual test cases override specific keys to verify validation branches.
func setBaselineEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://spacescale:spacescale@localhost:5432/spacescale?sslmode=disable")
	t.Setenv("BFF_JWT_SECRET", "test-secret")
	t.Setenv("INTERNAL_AUTH_SYNC_SECRET", "test-internal-auth-secret")
	t.Setenv("API_ENV_ENCRYPTION_KEY_ID", "test-key-v1")
	t.Setenv("API_ENV_ENCRYPTION_KEY", "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	// Use off mode by default so tests that do not care about hash mode are not
	// coupled to hash-secret requirements.
	t.Setenv("API_LOG_USER_AGENT_MODE", "off")
}

// TestLoadAppConfigMissingDatabaseURL verifies that startup fails fast when the
// database URL is missing.
func TestLoadAppConfigMissingDatabaseURL(t *testing.T) {
	setBaselineEnv(t)
	t.Setenv("DATABASE_URL", "")

	_, err := LoadFromEnv()
	require.EqualError(t, err, "missing required config DATABASE_URL")
}

// TestLoadAppConfigMissingJWTSecret verifies that startup fails fast when JWT
// verification secret is missing.
func TestLoadAppConfigMissingJWTSecret(t *testing.T) {
	setBaselineEnv(t)
	t.Setenv("BFF_JWT_SECRET", "")

	_, err := LoadFromEnv()
	require.EqualError(t, err, "missing required config BFF_JWT_SECRET")
}

// TestLoadAppConfigMissingInternalAuthSecret verifies startup fails fast when
// internal auth-sync shared secret is missing.
func TestLoadAppConfigMissingInternalAuthSecret(t *testing.T) {
	setBaselineEnv(t)
	t.Setenv("INTERNAL_AUTH_SYNC_SECRET", "")

	_, err := LoadFromEnv()
	require.EqualError(t, err, "missing required config INTERNAL_AUTH_SYNC_SECRET")
}

// TestLoadAppConfigMissingEnvEncryptionKeyID verifies startup fails fast when
// env value encryption key id is missing.
func TestLoadAppConfigMissingEnvEncryptionKeyID(t *testing.T) {
	setBaselineEnv(t)
	t.Setenv("API_ENV_ENCRYPTION_KEY_ID", "")

	_, err := LoadFromEnv()
	require.EqualError(t, err, "missing required config API_ENV_ENCRYPTION_KEY_ID")
}

// TestLoadAppConfigMissingEnvEncryptionKey verifies startup fails fast when
// env value encryption key material is missing.
func TestLoadAppConfigMissingEnvEncryptionKey(t *testing.T) {
	setBaselineEnv(t)
	t.Setenv("API_ENV_ENCRYPTION_KEY", "")

	_, err := LoadFromEnv()
	require.EqualError(t, err, "missing required config API_ENV_ENCRYPTION_KEY")
}

// TestLoadAppConfigInvalidEnvEncryptionKey verifies invalid key material is
// rejected during startup config loading.
func TestLoadAppConfigInvalidEnvEncryptionKey(t *testing.T) {
	setBaselineEnv(t)
	t.Setenv("API_ENV_ENCRYPTION_KEY", "not-base64")

	_, err := LoadFromEnv()
	require.EqualError(t, err, "invalid config API_ENV_ENCRYPTION_KEY: must be base64-encoded 32-byte key")
}

// TestLoadAppConfigParsesEnvEncryptionDecryptKeys verifies optional keyring
// config extends decrypt-capable key set while preserving active key selection.
func TestLoadAppConfigParsesEnvEncryptionDecryptKeys(t *testing.T) {
	setBaselineEnv(t)
	t.Setenv("API_ENV_ENCRYPTION_DECRYPT_KEYS", "legacy-v0:YWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWE=")

	cfg, err := LoadFromEnv()
	require.NoError(t, err)
	require.Equal(t, "test-key-v1", cfg.API.EnvEncryption.ActiveKeyID)
	require.Len(t, cfg.API.EnvEncryption.Keys, 2)
	require.Contains(t, cfg.API.EnvEncryption.Keys, "test-key-v1")
	require.Contains(t, cfg.API.EnvEncryption.Keys, "legacy-v0")
}

// TestLoadAppConfigInvalidEnvEncryptionDecryptKeys verifies invalid decrypt key
// entry formatting fails startup validation.
func TestLoadAppConfigInvalidEnvEncryptionDecryptKeys(t *testing.T) {
	setBaselineEnv(t)
	t.Setenv("API_ENV_ENCRYPTION_DECRYPT_KEYS", "broken-entry")

	_, err := LoadFromEnv()
	require.EqualError(t, err, "invalid config API_ENV_ENCRYPTION_DECRYPT_KEYS: expected comma-separated key_id:base64 pairs")
}

// TestLoadAppConfigHashModeRequiresSecret verifies that hash mode cannot start
// without dedicated HMAC secret key material.
func TestLoadAppConfigHashModeRequiresSecret(t *testing.T) {
	setBaselineEnv(t)
	t.Setenv("API_LOG_USER_AGENT_MODE", "hash")
	t.Setenv("API_LOG_USER_AGENT_HASH_SECRET", "")

	_, err := LoadFromEnv()
	require.EqualError(t, err, "API_LOG_USER_AGENT_HASH_SECRET is required when API_LOG_USER_AGENT_MODE=hash")
}

// TestLoadAppConfigClampsDBPoolBounds verifies DB pool normalization when min
// connections exceeds max connections.
func TestLoadAppConfigClampsDBPoolBounds(t *testing.T) {
	setBaselineEnv(t)
	t.Setenv("DB_MAX_CONNS", "10")
	t.Setenv("DB_MIN_CONNS", "20")

	cfg, err := LoadFromEnv()
	require.NoError(t, err)
	require.Equal(t, int32(10), cfg.Database.MaxConns)
	require.Equal(t, int32(10), cfg.Database.MinConns)
}

// TestLoadAppConfigBuildsTypedDefaults verifies default fallback behavior for
// optional settings and ensures top-level wiring fields are populated.
func TestLoadAppConfigBuildsTypedDefaults(t *testing.T) {
	setBaselineEnv(t)

	cfg, err := LoadFromEnv()
	require.NoError(t, err)
	require.Equal(t, defaultListenAddr, cfg.Addr)
	require.Equal(t, defaultDBMaxConns, cfg.Database.MaxConns)
	require.Equal(t, defaultDBMinConns, cfg.Database.MinConns)
	require.Equal(t, defaultAuthIssuer, cfg.API.Auth.Issuer)
	require.Equal(t, defaultAuthAudience, cfg.API.Auth.Audience)
	require.Equal(t, DefaultRateLimitConfig(), cfg.API.RateLimit)
	require.Equal(t, DefaultInternalGlobalRateLimitConfig(), cfg.API.InternalGlobalRateLimit)
	require.Equal(t, DefaultInternalIdentityRateLimitConfig(), cfg.API.InternalIdentityRateLimit)
	expectedLogPrivacy := DefaultLogPrivacyConfig()
	expectedLogPrivacy.UserAgentMode = UserAgentLogModeOff
	require.Equal(t, expectedLogPrivacy, cfg.API.LogPrivacy)
	require.Equal(t, "test-internal-auth-secret", cfg.API.InternalAuthSecret)
	require.Equal(t, defaultEnvReencryptBatchSize, cfg.API.EnvEncryption.ReencryptBatchSize)
	require.Equal(t, defaultEnvReencryptSweepPeriod, cfg.API.EnvEncryption.ReencryptSweepPeriod)
	require.Equal(t, defaultHTTPServerConfig().MaxBodyBytes, cfg.HTTPServer.MaxBodyBytes)
	require.Equal(t, defaultHTTPServerConfig().MaxHeaderBytes, cfg.HTTPServer.MaxHeaderBytes)
	require.Equal(t, defaultHTTPServerConfig().ReadHeaderTimeout, cfg.HTTPServer.ReadHeaderTimeout)
	require.Equal(t, defaultHTTPServerConfig().WriteTimeout, cfg.HTTPServer.WriteTimeout)
	require.Equal(t, defaultHTTPServerConfig().IdleTimeout, cfg.HTTPServer.IdleTimeout)
}

// TestLoadAppConfigParsesEnvReencryptSettings verifies optional env re-encryption
// worker tuning values are loaded into typed env-encryption config fields.
func TestLoadAppConfigParsesEnvReencryptSettings(t *testing.T) {
	setBaselineEnv(t)
	t.Setenv("API_ENV_ENCRYPTION_REENCRYPT_BATCH_SIZE", "250")
	t.Setenv("API_ENV_ENCRYPTION_REENCRYPT_SWEEP_PERIOD", "45s")

	cfg, err := LoadFromEnv()
	require.NoError(t, err)
	require.Equal(t, 250, cfg.API.EnvEncryption.ReencryptBatchSize)
	require.Equal(t, 45*time.Second, cfg.API.EnvEncryption.ReencryptSweepPeriod)
}

// TestLoadAppConfigParsesInternalRateLimits verifies internal route limiter env
// overrides are loaded into typed API config fields.
func TestLoadAppConfigParsesInternalRateLimits(t *testing.T) {
	setBaselineEnv(t)
	t.Setenv("API_INTERNAL_GLOBAL_RATE_LIMIT_REQUESTS", "20000")
	t.Setenv("API_INTERNAL_GLOBAL_RATE_LIMIT_WINDOW", "2m")
	t.Setenv("API_INTERNAL_IDENTITY_RATE_LIMIT_REQUESTS", "15")
	t.Setenv("API_INTERNAL_IDENTITY_RATE_LIMIT_WINDOW", "30s")

	cfg, err := LoadFromEnv()
	require.NoError(t, err)
	require.Equal(t, 20000, cfg.API.InternalGlobalRateLimit.Requests)
	require.Equal(t, 2*time.Minute, cfg.API.InternalGlobalRateLimit.Window)
	require.Equal(t, 15, cfg.API.InternalIdentityRateLimit.Requests)
	require.Equal(t, 30*time.Second, cfg.API.InternalIdentityRateLimit.Window)
}
