// This file centralizes API startup configuration loading.
// It parses environment variables into typed config structs, applies defaults,
// and validates required values so main.go can focus on process orchestration.

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

// AppConfig is the top-level startup configuration consumed by main.go.
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
// ReadHeaderTimeout, WriteTimeout, IdleTimeout, and MaxHeaderBytes are applied
// on http.Server directly.
type HTTPServerConfig struct {
	ReadHeaderTimeout time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	MaxBodyBytes      int64
	MaxHeaderBytes    int
}

// loadAppConfig reads and validates all startup configuration from environment.
// Required values fail fast; optional values default safely with warning logs.
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

// readRateLimitConfig loads per-user limiter settings with safe defaults.
func readRateLimitConfig() http_api.RateLimitConfig {
	defaults := http_api.DefaultRateLimitConfig()

	return http_api.RateLimitConfig{
		Requests: int(parseEnvInt32("API_USER_RATE_LIMIT_REQUESTS", int32(defaults.Requests))),
		Window:   parseEnvDuration("API_USER_RATE_LIMIT_WINDOW", defaults.Window),
	}
}

// readLogPrivacyConfig loads user-agent and panic-log privacy settings.
// Hash mode requires API_LOG_USER_AGENT_HASH_SECRET.
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
