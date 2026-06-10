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
	Enabled   bool
	JWTSecret string
	Issuer    string
	Audience  string
}

type controlEnv struct {
	Environment        string `env:"ENVIRONMENT"`
	NATSURL            string `env:"NATS_URL" envDefault:"nats://127.0.0.1:4222"`
	DatabaseURL        string `env:"DATABASE_URL"`
	ListenAddr         string `env:"LISTEN_ADDR" envDefault:":8080"`
	AuthEnabled        bool   `env:"AUTH_ENABLED" envDefault:"false"`
	AuthJWTSecret      string `env:"AUTH_JWT_SECRET"`
	AuthIssuer         string `env:"AUTH_ISSUER" envDefault:"api.spacescale.io"`
	AuthAudience       string `env:"AUTH_AUDIENCE" envDefault:"spacescale-api"`
	EnvEncryptionKeyID string `env:"API_ENV_ENCRYPTION_KEY_ID"`
	EnvEncryptionKey   string `env:"API_ENV_ENCRYPTION_KEY"`
}

type scaledEnv struct {
	Environment string `env:"ENVIRONMENT"`
	NATSURL     string `env:"NATS_URL" envDefault:"nats://127.0.0.1:4222"`
}

// LoadControl reads and validates config required by the control plane.
func LoadControl() (Control, error) {
	raw, err := env.ParseAs[controlEnv]()
	if err != nil {
		return Control{}, err
	}

	cfg := Control{
		Environment: strings.TrimSpace(raw.Environment),
		NATSURL:     strings.TrimSpace(raw.NATSURL),
		DatabaseURL: strings.TrimSpace(raw.DatabaseURL),
		ListenAddr:  strings.TrimSpace(raw.ListenAddr),
		Auth: AuthConfig{
			Enabled:   raw.AuthEnabled,
			JWTSecret: strings.TrimSpace(raw.AuthJWTSecret),
			Issuer:    strings.TrimSpace(raw.AuthIssuer),
			Audience:  strings.TrimSpace(raw.AuthAudience),
		},
		EnvEncryptionKeyID: strings.TrimSpace(raw.EnvEncryptionKeyID),
	}

	if err := cfg.normalizeAndValidate(strings.TrimSpace(raw.EnvEncryptionKey)); err != nil {
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
		Environment: strings.TrimSpace(raw.Environment),
		NATSURL:     strings.TrimSpace(raw.NATSURL),
	}
	if cfg.NATSURL == "" {
		cfg.NATSURL = defaultNATSURL
	}
	if err := normalizeAndValidateEnvironment(&cfg.Environment); err != nil {
		return Scaled{}, err
	}

	return cfg, nil
}

func (c *Control) normalizeAndValidate(rawEncryptionKey string) error {
	if err := normalizeAndValidateEnvironment(&c.Environment); err != nil {
		return err
	}
	if c.NATSURL == "" {
		c.NATSURL = defaultNATSURL
	}
	if c.DatabaseURL == "" {
		return errors.New("missing required config DATABASE_URL")
	}
	if c.ListenAddr == "" {
		c.ListenAddr = defaultListenAddr
	}
	if err := c.Auth.normalizeAndValidate(); err != nil {
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

func (a *AuthConfig) normalizeAndValidate() error {
	if a.Issuer == "" {
		a.Issuer = defaultAuthIssuer
	}
	if a.Audience == "" {
		a.Audience = defaultAuthAudience
	}
	if a.Enabled && a.JWTSecret == "" {
		return errors.New("missing required config AUTH_JWT_SECRET")
	}

	return nil
}

func normalizeAndValidateEnvironment(environment *string) error {
	trimmed := strings.TrimSpace(*environment)
	if trimmed == "" {
		*environment = developmentEnvironment
		return nil
	}
	if trimmed == productionEnvironment {
		*environment = productionEnvironment
		return nil
	}

	return errors.New("invalid config ENVIRONMENT")
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
