// Package config loads and validates process configuration from the environment.
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
)

const (
	envEncryptionKeyBytes = 32
	productionEnvironment = "production"

	defaultEnvironment     = "development"
	defaultNATSURL         = "nats://127.0.0.1:4222"
	defaultPort            = "8080"
	defaultFirecrackerBin  = "/usr/bin/firecracker"
	defaultJailerBin       = "/usr/bin/jailer"
	defaultGuestKernelPath = "/var/lib/spacescale/golden/vmlinux-v6.1.169-spacescale4-x86_64"
	defaultGuestRootFSPath = "/var/lib/spacescale/golden/guestd-rootfs-v0.1.3-x86_64-ext4"

	defaultAuthIssuer   = "spacescale-web-bff"
	defaultAuthAudience = "spacescale-api"
)

// Config is the normalized runtime configuration shared by scalecp and scaled.
type Config struct {
	Environment string
	NATSURL     string

	DatabaseURL     string
	Port            string // api server runtime port
	FirecrackerBin  string
	JailerBin       string
	GuestKernelPath string
	GuestRootFSPath string

	Auth               AuthConfig
	InternalAuthSecret string
	EnvEncryptionKeyID string
	EnvEncryptionKey   []byte
}

// AuthConfig contains BFF-issued JWT verification settings.
type AuthConfig struct {
	JWTSecret string
	Issuer    string
	Audience  string
}

// LoadScalecp reads, normalizes, and validates config required by the control plane.
func LoadScalecp() (Config, error) {
	cfg, err := loadFromEnv()
	if err != nil {
		return Config{}, err
	}

	return cfg.validateScalecp()
}

// LoadScaled reads, normalizes, and validates config required by the edge daemon.
func LoadScaled() (Config, error) {
	cfg, err := loadFromEnv()
	if err != nil {
		return Config{}, err
	}

	return cfg.validateScaled()
}

// loadFromEnv reads process configuration from environment variables and applies defaults.
func loadFromEnv() (Config, error) {
	envEncryptionKeyID, envEncryptionKey, err := readEnvEncryptionConfig()
	if err != nil {
		return Config{}, err
	}

	return Config{
		Environment: normalizeEnvironment(firstNonEmptyEnv("APP_ENV", "ENVIRONMENT")),
		NATSURL:     envStr("NATS_URL", defaultNATSURL),

		DatabaseURL:     strings.TrimSpace(os.Getenv("DATABASE_URL")),
		Port:            envStr("PORT", defaultPort),
		FirecrackerBin:  envStr("FIRECRACKER_BIN", defaultFirecrackerBin),
		JailerBin:       envStr("JAILER_BIN", defaultJailerBin),
		GuestKernelPath: envStr("GUEST_KERNEL_PATH", defaultGuestKernelPath),
		GuestRootFSPath: envStr("GUEST_ROOTFS_PATH", defaultGuestRootFSPath),

		Auth: AuthConfig{
			JWTSecret: strings.TrimSpace(os.Getenv("BFF_JWT_SECRET")),
			Issuer:    envStr("BFF_JWT_ISSUER", defaultAuthIssuer),
			Audience:  envStr("BFF_JWT_AUDIENCE", defaultAuthAudience),
		},
		InternalAuthSecret: strings.TrimSpace(os.Getenv("INTERNAL_AUTH_SYNC_SECRET")),
		EnvEncryptionKeyID: envEncryptionKeyID,
		EnvEncryptionKey:   envEncryptionKey,
	}.Normalized(), nil
}

// Normalized returns a copy with whitespace trimmed and defaults applied.
func (c Config) Normalized() Config {
	c.Environment = normalizeEnvironment(c.Environment)
	c.NATSURL = envOrDefault(strings.TrimSpace(c.NATSURL), defaultNATSURL)
	c.DatabaseURL = strings.TrimSpace(c.DatabaseURL)
	c.Port = envOrDefault(strings.TrimSpace(c.Port), defaultPort)
	c.FirecrackerBin = envOrDefault(strings.TrimSpace(c.FirecrackerBin), defaultFirecrackerBin)
	c.JailerBin = envOrDefault(strings.TrimSpace(c.JailerBin), defaultJailerBin)
	c.GuestKernelPath = envOrDefault(strings.TrimSpace(c.GuestKernelPath), defaultGuestKernelPath)
	c.GuestRootFSPath = envOrDefault(strings.TrimSpace(c.GuestRootFSPath), defaultGuestRootFSPath)
	c.Auth = c.Auth.Normalized()
	c.InternalAuthSecret = strings.TrimSpace(c.InternalAuthSecret)
	c.EnvEncryptionKeyID = strings.TrimSpace(c.EnvEncryptionKeyID)

	return c
}

// ListenAddr returns the HTTP listen address derived from Port.
func (c Config) ListenAddr() string {
	port := c.Normalized().Port
	if strings.Contains(port, ":") {
		return port
	}

	return ":" + port
}

// validateScalecp returns a Config or an error if any required control-plane field is missing.
func (c Config) validateScalecp() (Config, error) {
	c = c.Normalized()
	if c.DatabaseURL == "" {
		return Config{}, errors.New("missing required config DATABASE_URL")
	}
	if strings.TrimSpace(c.Auth.JWTSecret) == "" {
		return Config{}, errors.New("missing required config BFF_JWT_SECRET")
	}
	if c.InternalAuthSecret == "" {
		return Config{}, errors.New("missing required config INTERNAL_AUTH_SYNC_SECRET")
	}
	if c.EnvEncryptionKeyID == "" {
		return Config{}, errors.New("missing required config API_ENV_ENCRYPTION_KEY_ID")
	}
	if len(c.EnvEncryptionKey) == 0 {
		return Config{}, errors.New("missing required config API_ENV_ENCRYPTION_KEY")
	}
	if err := c.Auth.Validate(); err != nil {
		return Config{}, err
	}

	return c, nil
}

// validateScaled returns a Config or an error if any required edge-daemon field is missing.
func (c Config) validateScaled() (Config, error) {
	c = c.Normalized()
	if c.NATSURL == "" {
		return Config{}, errors.New("missing required config NATS_URL")
	}

	return c, nil
}

// Normalized returns a copy with auth defaults applied.
func (c AuthConfig) Normalized() AuthConfig {
	c.JWTSecret = strings.TrimSpace(c.JWTSecret)
	c.Issuer = envOrDefault(strings.TrimSpace(c.Issuer), defaultAuthIssuer)
	c.Audience = envOrDefault(strings.TrimSpace(c.Audience), defaultAuthAudience)

	return c
}

// Validate verifies auth settings required to accept BFF JWTs.
func (c AuthConfig) Validate() error {
	if c.JWTSecret == "" {
		return errors.New("auth config JWTSecret is required")
	}
	if c.Issuer == "" {
		return errors.New("auth config Issuer is required")
	}
	if c.Audience == "" {
		return errors.New("auth config Audience is required")
	}

	return nil
}

func readEnvEncryptionConfig() (string, []byte, error) {
	keyID := strings.TrimSpace(os.Getenv("API_ENV_ENCRYPTION_KEY_ID"))
	keyRaw := strings.TrimSpace(os.Getenv("API_ENV_ENCRYPTION_KEY"))
	if keyID == "" && keyRaw == "" {
		return "", nil, nil
	}
	if keyID != "" {
		if err := validateEnvEncryptionKeyID(keyID, "API_ENV_ENCRYPTION_KEY_ID"); err != nil {
			return "", nil, err
		}
	}
	if keyRaw == "" {
		return keyID, nil, nil
	}
	key, err := decodeEnvEncryptionKey(keyRaw, "API_ENV_ENCRYPTION_KEY")
	if err != nil {
		return "", nil, err
	}

	return keyID, key, nil
}

func decodeEnvEncryptionKey(raw, envName string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid config %s: must be base64-encoded 32-byte key", envName)
	}
	if len(key) != envEncryptionKeyBytes {
		return nil, fmt.Errorf("invalid config %s: must decode to 32 bytes", envName)
	}

	return append([]byte(nil), key...), nil
}

func validateEnvEncryptionKeyID(keyID, envName string) error {
	if strings.Contains(keyID, ":") || strings.ContainsAny(keyID, " \t\r\n") {
		return fmt.Errorf("invalid config %s: key id must not contain ':' or whitespace", envName)
	}

	return nil
}

func envStr(key, def string) string {
	return envOrDefault(firstNonEmptyEnv(key), def)
}

func envOrDefault(value, def string) string {
	if value == "" {
		return def
	}

	return value
}

func firstNonEmptyEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}

	return ""
}

func normalizeEnvironment(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case productionEnvironment, "prod":
		return productionEnvironment
	default:
		return defaultEnvironment
	}
}
