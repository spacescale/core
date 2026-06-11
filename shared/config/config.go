// Package config reads startup settings from environment variables.
package config

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/caarlos0/env/v11"
	"github.com/go-playground/validator/v10"
)

const envEncryptionKeyBytes = 32

var configValidator = validator.New(validator.WithRequiredStructEnabled())

// Control is the runtime configuration for the control plane.
type Control struct {
	Environment        string `validate:"required,oneof=development production"`
	NATSURL            string `validate:"required,url"`
	DatabaseURL        string `validate:"required"`
	ListenAddr         string `validate:"required"`
	WorkOS             WorkOSConfig
	EnvEncryptionKeyID string `validate:"required"`
	EnvEncryptionKey   []byte
}

// Scaled is the runtime configuration for the edge daemon.
type Scaled struct {
	Environment string `validate:"required,oneof=development production"`
	NATSURL     string `validate:"required,url"`
}

// WorkOSConfig holds the WorkOS settings used by the control plane.
type WorkOSConfig struct {
	APIKey               string `validate:"required"`
	ClientID             string `validate:"required"`
	CookiePassword       string `validate:"required,min=32"`
	RedirectURI          string `validate:"required,url"`
	PostLoginRedirectURI string `validate:"required,url"`
	LogoutRedirectURI    string `validate:"required,url"`
	CookieName           string `validate:"required"`
}

type controlEnv struct {
	Environment        string `env:"ENVIRONMENT"`
	NATSURL            string `env:"NATS_URL"`
	DatabaseURL        string `env:"DATABASE_URL"`
	ListenAddr         string `env:"LISTEN_ADDR"`
	EnvEncryptionKeyID string `env:"API_ENV_ENCRYPTION_KEY_ID"`
	EnvEncryptionKey   string `env:"API_ENV_ENCRYPTION_KEY"`

	WorkOSAPIKey               string `env:"WORKOS_API_KEY"`
	WorkOSClientID             string `env:"WORKOS_CLIENT_ID"`
	WorkOSCookiePassword       string `env:"WORKOS_COOKIE_PASSWORD"`
	WorkOSRedirectURI          string `env:"WORKOS_REDIRECT_URI"`
	WorkOSPostLoginRedirectURI string `env:"WORKOS_POST_LOGIN_REDIRECT_URI"`
	WorkOSLogoutRedirectURI    string `env:"WORKOS_LOGOUT_REDIRECT_URI"`
	WorkOSCookieName           string `env:"WORKOS_COOKIE_NAME"`
}

type scaledEnv struct {
	Environment string `env:"ENVIRONMENT"`
	NATSURL     string `env:"NATS_URL"`
}

// LoadControl reads control-plane config from the environment.
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
		WorkOS: WorkOSConfig{
			APIKey:               strings.TrimSpace(raw.WorkOSAPIKey),
			ClientID:             strings.TrimSpace(raw.WorkOSClientID),
			CookiePassword:       strings.TrimSpace(raw.WorkOSCookiePassword),
			RedirectURI:          strings.TrimSpace(raw.WorkOSRedirectURI),
			PostLoginRedirectURI: strings.TrimSpace(raw.WorkOSPostLoginRedirectURI),
			LogoutRedirectURI:    strings.TrimSpace(raw.WorkOSLogoutRedirectURI),
			CookieName:           strings.TrimSpace(raw.WorkOSCookieName),
		},
		EnvEncryptionKeyID: strings.TrimSpace(raw.EnvEncryptionKeyID),
	}
	if err := configValidator.Struct(cfg); err != nil {
		return Control{}, err
	}

	key, err := decodeEnvEncryptionKey(strings.TrimSpace(raw.EnvEncryptionKey), "API_ENV_ENCRYPTION_KEY")
	if err != nil {
		return Control{}, err
	}
	cfg.EnvEncryptionKey = key

	return cfg, nil
}

// LoadScaled reads edge-daemon config from the environment.
func LoadScaled() (Scaled, error) {
	raw, err := env.ParseAs[scaledEnv]()
	if err != nil {
		return Scaled{}, err
	}

	cfg := Scaled{
		Environment: strings.TrimSpace(raw.Environment),
		NATSURL:     strings.TrimSpace(raw.NATSURL),
	}
	if err := configValidator.Struct(cfg); err != nil {
		return Scaled{}, err
	}
	return cfg, nil
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
