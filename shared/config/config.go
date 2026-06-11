// Package config reads startup settings from environment variables.
// It turns raw env values into typed control-plane and edge configs.
// It trims input, applies defaults, and rejects invalid combinations early.
// It also handles the control-plane encryption key used at startup.
// The goal is to keep config loading predictable and easy to test.
// Callers should use the exported loaders instead of parsing env themselves.
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

	defaultWorkOSRedirectURI          = "http://localhost:8080/auth/callback"
	defaultWorkOSPostLoginRedirectURI = "http://localhost:3000"
	defaultWorkOSLogoutRedirectURI    = "http://localhost:3000"
	defaultWorkOSCookieName = "spacescale_session"
)

// Control is the runtime configuration for the control plane.
type Control struct {
	Environment        string
	NATSURL            string
	DatabaseURL        string
	ListenAddr         string
	Auth               AuthConfig
	WorkOS             WorkOSConfig
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

// WorkOSConfig holds the WorkOS settings used by the control plane.
// It stays separate from the other auth settings because it is optional.
type WorkOSConfig struct {
	APIKey               string
	ClientID             string
	CookiePassword       string
	RedirectURI          string
	PostLoginRedirectURI string
	LogoutRedirectURI    string
	CookieName           string
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

	// WorkOS
	WorkOSAPIKey               string `env:"WORKOS_API_KEY"`
	WorkOSClientID             string `env:"WORKOS_CLIENT_ID"`
	WorkOSCookiePassword       string `env:"WORKOS_COOKIE_PASSWORD"`
	WorkOSRedirectURI          string `env:"WORKOS_REDIRECT_URI"`
	WorkOSPostLoginRedirectURI string `env:"WORKOS_POST_LOGIN_REDIRECT_URI"`
	WorkOSLogoutRedirectURI    string `env:"WORKOS_LOGOUT_REDIRECT_URI"`
}

type scaledEnv struct {
	Environment string `env:"ENVIRONMENT"`
	NATSURL     string `env:"NATS_URL" envDefault:"nats://127.0.0.1:4222"`
}

// LoadControl reads control-plane config from the environment.
// It trims values, applies defaults, and validates required settings.
// The returned config is ready for startup or returns a clear error.
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
		WorkOS: WorkOSConfig{
			APIKey:               strings.TrimSpace(raw.WorkOSAPIKey),
			ClientID:             strings.TrimSpace(raw.WorkOSClientID),
			CookiePassword:       strings.TrimSpace(raw.WorkOSCookiePassword),
			RedirectURI:          strings.TrimSpace(raw.WorkOSRedirectURI),
			PostLoginRedirectURI: strings.TrimSpace(raw.WorkOSPostLoginRedirectURI),
			LogoutRedirectURI:    strings.TrimSpace(raw.WorkOSLogoutRedirectURI),
			CookieName:           defaultWorkOSCookieName,
		},
	}

	if err := cfg.normalizeAndValidate(strings.TrimSpace(raw.EnvEncryptionKey)); err != nil {
		return Control{}, err
	}

	return cfg, nil
}

// LoadScaled reads edge-daemon config from the environment.
// It trims values, applies defaults, and validates the environment name.
// The returned config is ready for immediate use or returns an error.
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

// normalizeAndValidate prepares control-plane config for startup.
// It fills in defaults, checks required values, and decodes the encryption key.
// The receiver is updated in place so the final config is complete.
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

	if err := c.WorkOS.normalizeAndValidate(c.Environment); err != nil {
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

// normalizeAndValidate applies auth defaults and checks required fields.
// It keeps local development simple unless auth is explicitly enabled.
// The helper stays unexported because it belongs to config loading.
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

// normalizeAndValidateEnvironment cleans up the ENVIRONMENT value.
// Empty input falls back to development and production stays unchanged.
// Any other value is rejected so startup fails fast.
func normalizeAndValidateEnvironment(environment *string) error {
	trimmed := strings.TrimSpace(*environment)
	if trimmed == "" {
		*environment = developmentEnvironment
		return nil
	}
	if trimmed == developmentEnvironment {
		*environment = developmentEnvironment
		return nil
	}
	if trimmed == productionEnvironment {
		*environment = productionEnvironment
		return nil
	}

	return errors.New("invalid config ENVIRONMENT")
}

// configured reports whether any WorkOS setting was provided.
// It is used to decide whether the WorkOS block should be validated.
// A single set field means the WorkOS config is active.
func (w *WorkOSConfig) configured() bool {
	return w.APIKey != "" ||
		w.ClientID != "" ||
		w.CookiePassword != "" ||
		w.RedirectURI != "" ||
		w.PostLoginRedirectURI != "" ||
		w.LogoutRedirectURI != ""
}

// normalizeAndValidate checks the WorkOS configuration block.
// It only runs when WorkOS has been configured at all.
// When active, it requires the keys, password, and redirect URIs.
func (w *WorkOSConfig) normalizeAndValidate(environment string) error {
	if !w.configured() {
		return nil
	}

	if w.APIKey == "" {
		return errors.New("missing required config WORKOS_API_KEY")
	}

	if w.ClientID == "" {
		return errors.New("missing required config WORKOS_CLIENT_ID")
	}

	if w.CookiePassword == "" {
		return errors.New("missing required config WORKOS_COOKIE_PASSWORD")
	}

	if len(w.CookiePassword) < 32 {
		return errors.New("invalid config WORKOS_COOKIE_PASSWORD: must be at least 32 characters")
	}

	if environment == developmentEnvironment {
		if w.RedirectURI == "" {
			w.RedirectURI = defaultWorkOSRedirectURI
		}
		if w.PostLoginRedirectURI == "" {
			w.PostLoginRedirectURI = defaultWorkOSPostLoginRedirectURI
		}
		if w.LogoutRedirectURI == "" {
			w.LogoutRedirectURI = defaultWorkOSLogoutRedirectURI
		}
	}

	if w.RedirectURI == "" {
		return errors.New("missing required config WORKOS_REDIRECT_URI")
	}

	if w.PostLoginRedirectURI == "" {
		return errors.New("missing required config WORKOS_POST_LOGIN_REDIRECT_URI")
	}

	if w.LogoutRedirectURI == "" {
		return errors.New("missing required config WORKOS_LOGOUT_REDIRECT_URI")
	}

	return nil
}
// decodeEnvEncryptionKey parses the control-plane encryption key from env.
// It expects base64 input that decodes to exactly 32 bytes.
// The returned slice is copied so callers can keep it safely.
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
