// Package config loads startup configuration from the environment.
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
	defaultEnvironment    = "development"
	defaultNATSURL        = "nats://127.0.0.1:4222"
	defaultPort           = "8080"
	defaultAuthIssuer     = "spacescale-web-bff"
	defaultAuthAudience   = "spacescale-api"
)

// Control is the runtime configuration for the control plane.
type Control struct {
	Environment string
	NATSURL     string

	DatabaseURL string
	Port        string

	Auth               AuthConfig
	InternalAuthSecret string
	EnvEncryptionKeyID string
	EnvEncryptionKey   []byte
}

// Scaled is the runtime configuration for the edge daemon.
type Scaled struct {
	Environment string
	NATSURL     string
}

// AuthConfig contains BFF-issued JWT verification settings.
type AuthConfig struct {
	JWTSecret string
	Issuer    string
	Audience  string
}

// LoadControl reads and validates config required by the control plane.
func LoadControl() (Control, error) {
	environment := strings.TrimSpace(os.Getenv("APP_ENV"))
	if environment == "" {
		environment = strings.TrimSpace(os.Getenv("ENVIRONMENT"))
	}
	switch strings.ToLower(environment) {
	case productionEnvironment, "prod":
		environment = productionEnvironment
	default:
		environment = defaultEnvironment
	}

	natsURL := strings.TrimSpace(os.Getenv("NATS_URL"))
	if natsURL == "" {
		natsURL = defaultNATSURL
	}

	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = defaultPort
	}

	issuer := strings.TrimSpace(os.Getenv("BFF_JWT_ISSUER"))
	if issuer == "" {
		issuer = defaultAuthIssuer
	}

	audience := strings.TrimSpace(os.Getenv("BFF_JWT_AUDIENCE"))
	if audience == "" {
		audience = defaultAuthAudience
	}

	keyID := strings.TrimSpace(os.Getenv("API_ENV_ENCRYPTION_KEY_ID"))
	keyRaw := strings.TrimSpace(os.Getenv("API_ENV_ENCRYPTION_KEY"))
	if keyID == "" {
		return Control{}, errors.New("missing required config API_ENV_ENCRYPTION_KEY_ID")
	}
	if keyRaw == "" {
		return Control{}, errors.New("missing required config API_ENV_ENCRYPTION_KEY")
	}
	key, err := decodeEnvEncryptionKey(keyRaw, "API_ENV_ENCRYPTION_KEY")
	if err != nil {
		return Control{}, err
	}

	cfg := Control{
		Environment: environment,
		NATSURL:     natsURL,
		DatabaseURL: strings.TrimSpace(os.Getenv("DATABASE_URL")),
		Port:        port,
		Auth: AuthConfig{
			JWTSecret: strings.TrimSpace(os.Getenv("BFF_JWT_SECRET")),
			Issuer:    issuer,
			Audience:  audience,
		},
		InternalAuthSecret: strings.TrimSpace(os.Getenv("INTERNAL_AUTH_SYNC_SECRET")),
		EnvEncryptionKeyID: keyID,
		EnvEncryptionKey:   key,
	}

	if cfg.DatabaseURL == "" {
		return Control{}, errors.New("missing required config DATABASE_URL")
	}
	if cfg.Auth.JWTSecret == "" {
		return Control{}, errors.New("missing required config BFF_JWT_SECRET")
	}
	if cfg.InternalAuthSecret == "" {
		return Control{}, errors.New("missing required config INTERNAL_AUTH_SYNC_SECRET")
	}

	return cfg, nil
}

// LoadScaled reads config required by the edge daemon.
func LoadScaled() Scaled {
	environment := strings.TrimSpace(os.Getenv("APP_ENV"))
	if environment == "" {
		environment = strings.TrimSpace(os.Getenv("ENVIRONMENT"))
	}
	switch strings.ToLower(environment) {
	case productionEnvironment, "prod":
		environment = productionEnvironment
	default:
		environment = defaultEnvironment
	}

	natsURL := strings.TrimSpace(os.Getenv("NATS_URL"))
	if natsURL == "" {
		natsURL = defaultNATSURL
	}

	return Scaled{
		Environment: environment,
		NATSURL:     natsURL,
	}
}

// ListenAddr returns the HTTP listen address derived from Port.
func (c Control) ListenAddr() string {
	port := strings.TrimSpace(c.Port)
	if port == "" {
		port = defaultPort
	}
	if strings.Contains(port, ":") {
		return port
	}

	return ":" + port
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
