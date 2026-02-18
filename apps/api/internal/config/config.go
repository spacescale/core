// This file centralizes API startup configuration loading.
// It parses environment variables into typed config structs, applies defaults,
// and validates required values so process entrypoints stay focused on wiring.

package config

import (
	"errors"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultListenAddr   = ":8080"              // default listen address used when ADDR is not set.
	defaultAuthIssuer   = "spacescale-web-bff" // expected JWT issuer when no explicit value is set.
	defaultAuthAudience = "spacescale-api"     // expected JWT audience when no explicit value is set.

	defaultDBMaxConns int32 = 20 // fallback maximum pgx pool connection count.
	defaultDBMinConns int32 = 5  // fallback minimum pgx pool connection count.

	defaultHTTPReadHeaderTimeout       = 5 * time.Second   // limits header read time per request.
	defaultHTTPWriteTimeout            = 30 * time.Second  // limits response write time per request.
	defaultHTTPIdleTimeout             = 120 * time.Second // limits keep-alive idle connection lifetime.
	defaultHTTPMaxBodyBytes      int64 = 1 << 20           // caps request body size at 1 MiB.
	defaultHTTPMaxHeaderBytes          = 1 << 20           // explicitly caps request header bytes at 1 MiB.
)

var (
	defaultDBMaxConnLifetime = time.Hour        // bounds lifetime of one DB connection.
	defaultDBMaxConnIdleTime = 30 * time.Minute // bounds how long an idle DB connection is retained.
)

// Config is the top-level startup configuration consumed by entrypoints.
type Config struct {
	Addr       string
	Database   DatabaseConfig
	API        APIConfig
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
// ReadHeaderTimeout, WriteTimeout, IdleTimeout, and MaxHeaderBytes are applied
// on http.Server directly.
type HTTPServerConfig struct {
	ReadHeaderTimeout time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	MaxBodyBytes      int64
	MaxHeaderBytes    int
}

// LoadFromEnv reads and validates all startup configuration from environment.
// Required values fail fast; optional values default safely with warning logs.
func LoadFromEnv() (Config, error) {
	databaseCfg, err := readDatabaseConfig()
	if err != nil {
		return Config{}, err
	}

	apiCfg, err := readAPIServerConfig()
	if err != nil {
		return Config{}, err
	}

	return Config{
		Addr:       envStr("ADDR", defaultListenAddr),
		Database:   databaseCfg,
		API:        apiCfg,
		HTTPServer: defaultHTTPServerConfig(),
	}, nil
}

// readAPIServerConfig loads API runtime settings used by server middleware and
// internal endpoint protection.
func readAPIServerConfig() (APIConfig, error) {
	authCfg, err := readAuthConfig()
	if err != nil {
		return APIConfig{}, err
	}

	logPrivacyCfg, err := readLogPrivacyConfig()
	if err != nil {
		return APIConfig{}, err
	}

	internalAuthSecret := strings.TrimSpace(envStr("INTERNAL_AUTH_SYNC_SECRET", ""))
	if internalAuthSecret == "" {
		return APIConfig{}, errors.New("missing required config INTERNAL_AUTH_SYNC_SECRET")
	}

	return APIConfig{
		Auth:                      authCfg,
		RateLimit:                 readRateLimitConfig(),
		InternalIdentityRateLimit: readInternalIdentityRateLimitConfig(),
		LogPrivacy:                logPrivacyCfg,
		InternalAuthSecret:        internalAuthSecret,
	}, nil
}

// defaultHTTPServerConfig returns conservative HTTP hardening defaults.
func defaultHTTPServerConfig() HTTPServerConfig {
	return HTTPServerConfig{
		ReadHeaderTimeout: defaultHTTPReadHeaderTimeout,
		WriteTimeout:      defaultHTTPWriteTimeout,
		IdleTimeout:       defaultHTTPIdleTimeout,
		MaxBodyBytes:      defaultHTTPMaxBodyBytes,
		MaxHeaderBytes:    defaultHTTPMaxHeaderBytes,
	}
}

// readDatabaseConfig loads database URL and pool settings.
// It clamps min connections when DB_MIN_CONNS exceeds DB_MAX_CONNS.
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

// readAuthConfig loads auth verification settings and validates them.
func readAuthConfig() (AuthConfig, error) {
	jwtSecret := strings.TrimSpace(envStr("BFF_JWT_SECRET", ""))
	if jwtSecret == "" {
		return AuthConfig{}, errors.New("missing required config BFF_JWT_SECRET")
	}

	cfg := AuthConfig{
		JWTSecret: jwtSecret,
		Issuer:    envStr("BFF_JWT_ISSUER", defaultAuthIssuer),
		Audience:  envStr("BFF_JWT_AUDIENCE", defaultAuthAudience),
	}
	if err := cfg.Validate(); err != nil {
		return AuthConfig{}, err
	}

	return cfg, nil
}

// readRateLimitConfig loads per-user limiter settings with safe defaults.
func readRateLimitConfig() RateLimitConfig {
	defaults := DefaultRateLimitConfig()

	return RateLimitConfig{
		Requests: int(parseEnvInt32("API_USER_RATE_LIMIT_REQUESTS", int32(defaults.Requests))),
		Window:   parseEnvDuration("API_USER_RATE_LIMIT_WINDOW", defaults.Window),
	}
}

// readInternalIdentityRateLimitConfig loads per-identity limiter settings for
// trusted internal auth-sync requests.
func readInternalIdentityRateLimitConfig() RateLimitConfig {
	defaults := DefaultInternalIdentityRateLimitConfig()

	return RateLimitConfig{
		Requests: int(parseEnvInt32("API_INTERNAL_IDENTITY_RATE_LIMIT_REQUESTS", int32(defaults.Requests))),
		Window:   parseEnvDuration("API_INTERNAL_IDENTITY_RATE_LIMIT_WINDOW", defaults.Window),
	}
}

// readLogPrivacyConfig loads user-agent and panic-log privacy settings.
// Hash mode requires API_LOG_USER_AGENT_HASH_SECRET.
func readLogPrivacyConfig() (LogPrivacyConfig, error) {
	defaults := DefaultLogPrivacyConfig()
	modeRaw := strings.TrimSpace(strings.ToLower(envStr("API_LOG_USER_AGENT_MODE", string(defaults.UserAgentMode))))

	cfg := LogPrivacyConfig{
		UserAgentHashSecret: strings.TrimSpace(envStr("API_LOG_USER_AGENT_HASH_SECRET", "")),
		UserAgentMaxLen:     int(parseEnvInt32("API_LOG_USER_AGENT_MAX_LEN", int32(defaults.UserAgentMaxLen))),
		PanicValueMaxLen:    int(parseEnvInt32("API_LOG_PANIC_VALUE_MAX_LEN", int32(defaults.PanicValueMaxLen))),
		IncludeStackTrace:   parseEnvBool("API_LOG_STACK_TRACE", defaults.IncludeStackTrace),
	}

	switch UserAgentLogMode(modeRaw) {
	case UserAgentLogModeHash,
		UserAgentLogModeTruncate,
		UserAgentLogModeOff:
		cfg.UserAgentMode = UserAgentLogMode(modeRaw)
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
	if cfg.UserAgentMode == UserAgentLogModeHash && cfg.UserAgentHashSecret == "" {
		return LogPrivacyConfig{}, errors.New("API_LOG_USER_AGENT_HASH_SECRET is required when API_LOG_USER_AGENT_MODE=hash")
	}

	return cfg, nil
}

// envStr returns an environment value or fallback default.
func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// logDefaultedEnv emits a consistent warning when invalid env input is defaulted.
func logDefaultedEnv(key, raw string, def any) {
	slog.Warn(
		"invalid env value; using default",
		"event", "startup_config_defaulted",
		"key", key,
		"raw_value", raw,
		"default_value", def,
	)
}

// parseEnvInt32 parses an int32 env value with default fallback and warning.
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

// parseEnvDuration parses a duration env value with default fallback and warning.
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

// parseEnvBool parses a bool env value with default fallback and warning.
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
