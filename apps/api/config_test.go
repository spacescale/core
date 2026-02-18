// This file tests startup configuration loading in package main.
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

package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// setBaselineEnv seeds required environment keys for successful config loading.
// Individual test cases override specific keys to verify validation branches.
func setBaselineEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://spacescale:spacescale@localhost:5432/spacescale?sslmode=disable")
	t.Setenv("BFF_JWT_SECRET", "test-secret")
	// Use off mode by default so tests that do not care about hash mode are not
	// coupled to hash-secret requirements.
	t.Setenv("API_LOG_USER_AGENT_MODE", "off")
}

// TestLoadAppConfigMissingDatabaseURL verifies that startup fails fast when the
// database URL is missing.
func TestLoadAppConfigMissingDatabaseURL(t *testing.T) {
	setBaselineEnv(t)
	t.Setenv("DATABASE_URL", "")

	_, err := loadAppConfig()
	require.EqualError(t, err, "missing required config DATABASE_URL")
}

// TestLoadAppConfigMissingJWTSecret verifies that startup fails fast when JWT
// verification secret is missing.
func TestLoadAppConfigMissingJWTSecret(t *testing.T) {
	setBaselineEnv(t)
	t.Setenv("BFF_JWT_SECRET", "")

	_, err := loadAppConfig()
	require.EqualError(t, err, "missing required config BFF_JWT_SECRET")
}

// TestLoadAppConfigHashModeRequiresSecret verifies that hash mode cannot start
// without dedicated HMAC secret key material.
func TestLoadAppConfigHashModeRequiresSecret(t *testing.T) {
	setBaselineEnv(t)
	t.Setenv("API_LOG_USER_AGENT_MODE", "hash")
	t.Setenv("API_LOG_USER_AGENT_HASH_SECRET", "")

	_, err := loadAppConfig()
	require.EqualError(t, err, "API_LOG_USER_AGENT_HASH_SECRET is required when API_LOG_USER_AGENT_MODE=hash")
}

// TestLoadAppConfigClampsDBPoolBounds verifies DB pool normalization when min
// connections exceeds max connections.
func TestLoadAppConfigClampsDBPoolBounds(t *testing.T) {
	setBaselineEnv(t)
	t.Setenv("DB_MAX_CONNS", "10")
	t.Setenv("DB_MIN_CONNS", "20")

	cfg, err := loadAppConfig()
	require.NoError(t, err)
	require.Equal(t, int32(10), cfg.Database.MaxConns)
	require.Equal(t, int32(10), cfg.Database.MinConns)
}

// TestLoadAppConfigBuildsTypedDefaults verifies default fallback behavior for
// optional settings and ensures top-level wiring fields are populated.
func TestLoadAppConfigBuildsTypedDefaults(t *testing.T) {
	setBaselineEnv(t)

	cfg, err := loadAppConfig()
	require.NoError(t, err)
	require.Equal(t, defaultListenAddr, cfg.Addr)
	require.Equal(t, defaultDBMaxConns, cfg.Database.MaxConns)
	require.Equal(t, defaultDBMinConns, cfg.Database.MinConns)
	require.Equal(t, defaultHTTPServerConfig().MaxBodyBytes, cfg.HTTPServer.MaxBodyBytes)
	require.Equal(t, defaultHTTPServerConfig().MaxHeaderBytes, cfg.HTTPServer.MaxHeaderBytes)
	require.Equal(t, defaultHTTPServerConfig().ReadHeaderTimeout, cfg.HTTPServer.ReadHeaderTimeout)
	require.Equal(t, defaultHTTPServerConfig().WriteTimeout, cfg.HTTPServer.WriteTimeout)
	require.Equal(t, defaultHTTPServerConfig().IdleTimeout, cfg.HTTPServer.IdleTimeout)
}
