// This file centralizes startup configuration loading for the API process.
//
// Why this file exists:
// - Keep main.go focused on orchestration (boot, wire, serve, shutdown).
// - Parse environment variables once into typed configuration structs.
// - Apply defaults and normalization in one place for predictable runtime behavior.
// - Emit consistent warning logs when invalid env values are defaulted.
//
// Configuration domains loaded here:
// - process/server settings (listen address and HTTP safety limits)
// - database pool settings
// - auth validation settings
// - API rate limiting
// - log privacy settings (user-agent and panic logging policy)

package main

import (
	"errors"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/t0gun/spacescale/internal/http_api"
)

const (
	// defaultListenAddr is used when ADDR is not set.
	defaultListenAddr = ":8080"
	// defaultAuthIssuer is the expected JWT issuer when no explicit value is set.
	defaultAuthIssuer = "spacescale-web-bff"
	// defaultAuthAudience is the expected JWT audience when no explicit value is set.
	defaultAuthAudience = "spacescale-api"

	// defaultDBMaxConns is the fallback maximum connection count for pgx pool.
	defaultDBMaxConns int32 = 20
	// defaultDBMinConns is the fallback minimum connection count for pgx pool.
	defaultDBMinConns int32 = 5
)

var (
	// defaultDBMaxConnLifetime bounds how long a single DB connection can live.
	defaultDBMaxConnLifetime = time.Hour
	// defaultDBMaxConnIdleTime bounds how long an idle DB connection is retained.
	defaultDBMaxConnIdleTime = 30 * time.Minute
)

// AppConfig is the top-level typed startup configuration used by main.go.
//
// Design intent:
// - Load once at startup.
// - Pass scoped sub-configs to the components that own each behavior.
// - Keep configuration boundaries explicit and testable.
type AppConfig struct {
	Addr       string
	Database   DatabaseConfig
	Auth       http_api.AuthConfig
	RateLimit  http_api.RateLimitConfig
	LogPrivacy http_api.LogPrivacyConfig
	HTTPServer HTTPServerConfig
}

// DatabaseConfig defines connection and pool settings used to initialize pgx.
//
// URL is required. Pool values are normalized during config loading.
type DatabaseConfig struct {
	URL             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
}

// HTTPServerConfig defines server-level safety and connection-lifecycle settings.
//
// MaxBodyBytes is applied via http.MaxBytesHandler wrapper.
// ReadHeaderTimeout and IdleTimeout are applied on http.Server directly.
type HTTPServerConfig struct {
	ReadHeaderTimeout time.Duration
	IdleTimeout       time.Duration
	MaxBodyBytes      int64
}

// loadAppConfig reads and validates all startup configuration from environment.
//
// Return behavior:
// - Returns a fully typed AppConfig when required values are present and valid.
// - Returns error when required values are missing or cross-field validation fails.
//
// Startup policy:
// - Required values fail fast.
// - Optional values default safely.
// - Invalid optional values default and emit warning logs.
func loadAppConfig() (AppConfig, error) {
	databaseCfg, err := readDatabaseConfig()
	if err != nil {
		return AppConfig{}, err
	}

	authCfg, err := readAuthConfig()
	if err != nil {
		return AppConfig{}, err
	}

	rateLimitCfg := readRateLimitConfig()
	logPrivacyCfg, err := readLogPrivacyConfig()
	if err != nil {
		return AppConfig{}, err
	}

	return AppConfig{
		Addr:       envStr("ADDR", defaultListenAddr),
		Database:   databaseCfg,
		Auth:       authCfg,
		RateLimit:  rateLimitCfg,
		LogPrivacy: logPrivacyCfg,
		HTTPServer: defaultHTTPServerConfig(),
	}, nil
}

// defaultHTTPServerConfig returns hardening defaults for HTTP server behavior.
//
// These defaults are intentionally conservative and can be promoted to env-backed
// config later if runtime tuning needs grow.
func defaultHTTPServerConfig() HTTPServerConfig {
	return HTTPServerConfig{
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxBodyBytes:      1 << 20,
	}
}

// readDatabaseConfig loads database URL and pool settings from environment.
//
// Required key:
// - DATABASE_URL
//
// Optional keys:
// - DB_MAX_CONNS
// - DB_MIN_CONNS
//
// Normalization:
// - If DB_MIN_CONNS > DB_MAX_CONNS, min is clamped to max and warning is logged.
func readDatabaseConfig() (DatabaseConfig, error) {
	url := strings.TrimSpace(envStr("DATABASE_URL", ""))
	if url == "" {
		return DatabaseConfig{}, errors.New("missing required config DATABASE_URL")
	}

	maxConns := parseEnvInt32("DB_MAX_CONNS", defaultDBMaxConns)
	minConns := parseEnvInt32("DB_MIN_CONNS", defaultDBMinConns)
	if minConns > maxConns {
		slog.Warn(
			"invalid db pool bounds; clamping min to max",
			"event", "startup_config_defaulted",
			"key", "DB_MIN_CONNS",
			"min_conns", minConns,
			"max_conns", maxConns,
		)
		minConns = maxConns
	}

	return DatabaseConfig{
		URL:             url,
		MaxConns:        maxConns,
		MinConns:        minConns,
		MaxConnLifetime: defaultDBMaxConnLifetime,
		MaxConnIdleTime: defaultDBMaxConnIdleTime,
	}, nil
}

// readAuthConfig loads API auth verification settings from environment.
//
// Required key:
// - BFF_JWT_SECRET
//
// Optional keys:
// - BFF_JWT_ISSUER
// - BFF_JWT_AUDIENCE
//
// Validation delegates to http_api.AuthConfig.Validate for consistency with
// middleware expectations.
func readAuthConfig() (http_api.AuthConfig, error) {
	jwtSecret := strings.TrimSpace(envStr("BFF_JWT_SECRET", ""))
	if jwtSecret == "" {
		return http_api.AuthConfig{}, errors.New("missing required config BFF_JWT_SECRET")
	}

	cfg := http_api.AuthConfig{
		JWTSecret: jwtSecret,
		Issuer:    envStr("BFF_JWT_ISSUER", defaultAuthIssuer),
		Audience:  envStr("BFF_JWT_AUDIENCE", defaultAuthAudience),
	}
	if err := cfg.Validate(); err != nil {
		return http_api.AuthConfig{}, err
	}

	return cfg, nil
}

// readRateLimitConfig loads API per-user limiter settings from environment.
//
// Supported keys:
// - API_USER_RATE_LIMIT_REQUESTS
// - API_USER_RATE_LIMIT_WINDOW
//
// Invalid values default safely and emit startup warnings via parse helpers.
func readRateLimitConfig() http_api.RateLimitConfig {
	defaults := http_api.DefaultRateLimitConfig()

	return http_api.RateLimitConfig{
		Requests: int(parseEnvInt32("API_USER_RATE_LIMIT_REQUESTS", int32(defaults.Requests))),
		Window:   parseEnvDuration("API_USER_RATE_LIMIT_WINDOW", defaults.Window),
	}
}

// readLogPrivacyConfig loads request-log privacy behavior from environment.
//
// Supported keys:
// - API_LOG_USER_AGENT_MODE: hash | truncate | off
// - API_LOG_USER_AGENT_HASH_SECRET: required when mode is hash
// - API_LOG_USER_AGENT_MAX_LEN
// - API_LOG_PANIC_VALUE_MAX_LEN
// - API_LOG_STACK_TRACE
//
// Validation behavior:
// - Unknown mode defaults to package default with warning.
// - Non-positive numeric values default to package defaults.
// - Hash mode requires non-empty secret and returns an error when missing.
func readLogPrivacyConfig() (http_api.LogPrivacyConfig, error) {
	defaults := http_api.DefaultLogPrivacyConfig()
	modeRaw := strings.TrimSpace(strings.ToLower(envStr("API_LOG_USER_AGENT_MODE", string(defaults.UserAgentMode))))

	cfg := http_api.LogPrivacyConfig{
		UserAgentHashSecret: strings.TrimSpace(envStr("API_LOG_USER_AGENT_HASH_SECRET", "")),
		UserAgentMaxLen:     int(parseEnvInt32("API_LOG_USER_AGENT_MAX_LEN", int32(defaults.UserAgentMaxLen))),
		PanicValueMaxLen:    int(parseEnvInt32("API_LOG_PANIC_VALUE_MAX_LEN", int32(defaults.PanicValueMaxLen))),
		IncludeStackTrace:   parseEnvBool("API_LOG_STACK_TRACE", defaults.IncludeStackTrace),
	}

	switch http_api.UserAgentLogMode(modeRaw) {
	case http_api.UserAgentLogModeHash,
		http_api.UserAgentLogModeTruncate,
		http_api.UserAgentLogModeOff:
		cfg.UserAgentMode = http_api.UserAgentLogMode(modeRaw)
	default:
		logDefaultedEnv("API_LOG_USER_AGENT_MODE", modeRaw, string(defaults.UserAgentMode))
		cfg.UserAgentMode = defaults.UserAgentMode
	}

	if cfg.UserAgentMaxLen <= 0 {
		cfg.UserAgentMaxLen = defaults.UserAgentMaxLen
	}
	if cfg.PanicValueMaxLen <= 0 {
		cfg.PanicValueMaxLen = defaults.PanicValueMaxLen
	}
	if cfg.UserAgentMode == http_api.UserAgentLogModeHash && cfg.UserAgentHashSecret == "" {
		return http_api.LogPrivacyConfig{}, errors.New("API_LOG_USER_AGENT_HASH_SECRET is required when API_LOG_USER_AGENT_MODE=hash")
	}

	return cfg, nil
}

// envStr returns an environment value or fallback default.
//
// It preserves compact call sites while keeping default values explicit at the
// usage point.
func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// logDefaultedEnv emits a standardized startup warning when an invalid env value
// is supplied and runtime falls back to a safe default.
//
// Keeping this function centralized avoids duplicated warning payloads and makes
// startup diagnostics consistent across all config readers.
func logDefaultedEnv(key, raw string, def any) {
	slog.Warn(
		"invalid env value; using default",
		"event", "startup_config_defaulted",
		"key", key,
		"raw_value", raw,
		"default_value", def,
	)
}

// parseEnvInt32 parses an environment variable as int32 with default fallback.
//
// Behavior:
// - Returns def when env is empty.
// - Returns def and logs warning when value is invalid or out of int32 range.
// - Returns parsed value when valid.
func parseEnvInt32(key string, def int32) int32 {
	const (
		int32Max = int64(^uint32(0) >> 1)
		int32Min = -int32Max - 1
	)

	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}

	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		logDefaultedEnv(key, raw, def)
		return def
	}
	if n < int32Min || n > int32Max {
		logDefaultedEnv(key, raw, def)
		return def
	}

	return int32(n)
}

// parseEnvDuration parses an environment variable as Go duration with fallback.
//
// Behavior:
// - Returns def when env is empty.
// - Returns def and logs warning when duration is invalid or non-positive.
// - Returns parsed duration when valid.
func parseEnvDuration(key string, def time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}

	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		logDefaultedEnv(key, raw, def.String())
		return def
	}

	return d
}

// parseEnvBool parses an environment variable as bool with default fallback.
//
// Behavior:
// - Returns def when env is empty.
// - Returns def and logs warning when value cannot be parsed as bool.
// - Returns parsed bool when valid.
func parseEnvBool(key string, def bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}

	b, err := strconv.ParseBool(raw)
	if err != nil {
		logDefaultedEnv(key, raw, def)
		return def
	}

	return b
}
