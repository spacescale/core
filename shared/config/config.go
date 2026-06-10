// Package config loads startup configuration from the environment.
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/caarlos0/env/v11"
)

const (
	envEncryptionKeyBytes = 32

	developmentEnvironment = "development"
	productionEnvironment  = "production"

	defaultNATSURL      = "nats://127.0.0.1:4222"
	defaultListenAddr   = ":8080"
	defaultAuthIssuer   = "api.spacescale.io"
	defaultAuthAudience = "spacescale-api"
)

// AuthMode controls whether control-plane auth is enforced.
type AuthMode string

const (
	// AuthModeDisabled bypasses auth for local development.
	AuthModeDisabled AuthMode = "disabled"
	// AuthModeEnabled requires configured auth credentials.
	AuthModeEnabled  AuthMode = "enabled"

	defaultAuthMode = AuthModeDisabled
)

// Control is the runtime configuration for the control plane.
type Control struct {
	Environment        string
	NATSURL            string
	DatabaseURL        string
	ListenAddr         string
	Auth               AuthConfig
	EnvEncryptionKeyID string
	EnvEncryptionKey   []byte
}

// Scaled is the runtime configuration for the edge daemon.
type Scaled struct {
	Environment string
	NATSURL     string
}

// AuthConfig contains first-party control-plane auth settings.
type AuthConfig struct {
	Mode      AuthMode
	JWTSecret string
	Issuer    string
	Audience  string
}

type controlEnv struct {
	EnvironmentRaw     string `env:"ENVIRONMENT"`
	NATSURL            string `env:"NATS_URL" envDefault:"nats://127.0.0.1:4222"`
	DatabaseURL        string `env:"DATABASE_URL"`
	ListenAddr         string `env:"LISTEN_ADDR" envDefault:":8080"`
	AuthMode           string `env:"AUTH_MODE" envDefault:"disabled"`
	AuthJWTSecret      string `env:"AUTH_JWT_SECRET"`
	AuthIssuer         string `env:"AUTH_ISSUER" envDefault:"api.spacescale.io"`
	AuthAudience       string `env:"AUTH_AUDIENCE" envDefault:"spacescale-api"`
	EnvEncryptionKeyID string `env:"API_ENV_ENCRYPTION_KEY_ID"`
	EnvEncryptionKey   string `env:"API_ENV_ENCRYPTION_KEY"`
}

type scaledEnv struct {
	EnvironmentRaw string `env:"ENVIRONMENT"`
	NATSURL        string `env:"NATS_URL" envDefault:"nats://127.0.0.1:4222"`
}

// LoadControl reads and validates config required by the control plane.
func LoadControl() (Control, error) {
	raw, err := env.ParseAs[controlEnv]()
	if err != nil {
		return Control{}, err
	}

	cfg := Control{
		Environment: normalizeEnvironment(raw.EnvironmentRaw),
		NATSURL:     strings.TrimSpace(raw.NATSURL),
		DatabaseURL: strings.TrimSpace(raw.DatabaseURL),
		ListenAddr:  strings.TrimSpace(raw.ListenAddr),
		Auth: AuthConfig{
			Mode:      AuthMode(strings.ToLower(strings.TrimSpace(raw.AuthMode))),
			JWTSecret: strings.TrimSpace(raw.AuthJWTSecret),
			Issuer:    strings.TrimSpace(raw.AuthIssuer),
			Audience:  strings.TrimSpace(raw.AuthAudience),
		},
		EnvEncryptionKeyID: strings.TrimSpace(raw.EnvEncryptionKeyID),
	}

	if err := cfg.finalize(strings.TrimSpace(raw.EnvEncryptionKey)); err != nil {
		return Control{}, err
	}

	return cfg, nil
}

// LoadScaled reads config required by the edge daemon.
func LoadScaled() (Scaled, error) {
	raw, err := env.ParseAs[scaledEnv]()
	if err != nil {
		return Scaled{}, err
	}

	cfg := Scaled{
		Environment: normalizeEnvironment(raw.EnvironmentRaw),
		NATSURL:     strings.TrimSpace(raw.NATSURL),
	}
	if cfg.NATSURL == "" {
		cfg.NATSURL = defaultNATSURL
	}

	return cfg, nil
}

func (c *Control) finalize(rawEncryptionKey string) error {
	if c.NATSURL == "" {
		c.NATSURL = defaultNATSURL
	}
	if c.DatabaseURL == "" {
		return errors.New("missing required config DATABASE_URL")
	}
	if c.ListenAddr == "" {
		c.ListenAddr = defaultListenAddr
	}
	if err := c.Auth.finalize(); err != nil {
		return err
	}
	if c.EnvEncryptionKeyID == "" {
		return errors.New("missing required config API_ENV_ENCRYPTION_KEY_ID")
	}

	key, err := decodeEnvEncryptionKey(rawEncryptionKey, "API_ENV_ENCRYPTION_KEY")
	if err != nil {
		return err
	}
	c.EnvEncryptionKey = key

	return nil
}

func (a *AuthConfig) finalize() error {
	if a.Mode == "" {
		a.Mode = defaultAuthMode
	}
	switch a.Mode {
	case AuthModeDisabled, AuthModeEnabled:
	default:
		return fmt.Errorf("invalid config AUTH_MODE: %s", a.Mode)
	}

	if a.Issuer == "" {
		a.Issuer = defaultAuthIssuer
	}
	if a.Audience == "" {
		a.Audience = defaultAuthAudience
	}
	if a.Mode != AuthModeDisabled && a.JWTSecret == "" {
		return errors.New("missing required config AUTH_JWT_SECRET")
	}

	return nil
}

func normalizeEnvironment(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return developmentEnvironment
	}
	return productionEnvironment
}

func decodeEnvEncryptionKey(raw, envName string) ([]byte, error) {
	if raw == "" {
		return nil, fmt.Errorf("missing required config %s", envName)
	}

	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid config %s: must be base64-encoded 32-byte key", envName)
	}
	if len(key) != envEncryptionKeyBytes {
		return nil, fmt.Errorf("invalid config %s: must decode to 32 bytes", envName)
	}

	return append([]byte(nil), key...), nil
}
