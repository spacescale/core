package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
)

const (
	defaultEnvironment    = "development"
	defaultNATSURL        = "nats://127.0.0.1:4222"
	defaultPort           = "8080"
	defaultFirecrackerBin = "/usr/bin/firecracker"

	defaultAuthIssuer   = "spacescale-web-bff"
	defaultAuthAudience = "spacescale-api"
)

type Config struct {
	Environment string
	NATSURL     string

	DatabaseURL    string
	Port           string // api server runtime port
	FirecrackerBin string

	Auth               AuthConfig
	InternalAuthSecret string
	EnvEncryptionKeyID string
	EnvEncryptionKey   []byte
}

type AuthConfig struct {
	JWTSecret string
	Issuer    string
	Audience  string
}

func Load() (Config, error) {
	envEncryptionKeyID, envEncryptionKey, err := readEnvEncryptionConfig()
	if err != nil {
		return Config{}, err
	}

	return Config{
		Environment: normalizeEnvironment(firstNonEmptyEnv("APP_ENV", "ENVIRONMENT")),
		NATSURL:     envStr("NATS_URL", defaultNATSURL),

		DatabaseURL:    strings.TrimSpace(os.Getenv("DATABASE_URL")),
		Port:           envStr("PORT", defaultPort),
		FirecrackerBin: envStr("FIRECRACKER_BIN", defaultFirecrackerBin),

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

func (c Config) Normalized() Config {
	c.Environment = normalizeEnvironment(c.Environment)
	c.NATSURL = envOrDefault(strings.TrimSpace(c.NATSURL), defaultNATSURL)
	c.DatabaseURL = strings.TrimSpace(c.DatabaseURL)
	c.Port = envOrDefault(strings.TrimSpace(c.Port), defaultPort)
	c.FirecrackerBin = envOrDefault(strings.TrimSpace(c.FirecrackerBin), defaultFirecrackerBin)
	c.Auth = c.Auth.Normalized()
	c.InternalAuthSecret = strings.TrimSpace(c.InternalAuthSecret)
	c.EnvEncryptionKeyID = strings.TrimSpace(c.EnvEncryptionKeyID)
	return c
}

func (c Config) ListenAddr() string {
	port := c.Normalized().Port
	if strings.Contains(port, ":") {
		return port
	}
	return ":" + port
}

func (c Config) ValidateScalecp() (Config, error) {
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

func (c Config) ValidateScaled() (Config, error) {
	c = c.Normalized()
	if c.NATSURL == "" {
		return Config{}, errors.New("missing required config NATS_URL")
	}
	return c, nil
}

func (c AuthConfig) Normalized() AuthConfig {
	c.JWTSecret = strings.TrimSpace(c.JWTSecret)
	c.Issuer = envOrDefault(strings.TrimSpace(c.Issuer), defaultAuthIssuer)
	c.Audience = envOrDefault(strings.TrimSpace(c.Audience), defaultAuthAudience)
	return c
}

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
	if len(key) != 32 {
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
	case "production", "prod":
		return "production"
	default:
		return defaultEnvironment
	}
}
