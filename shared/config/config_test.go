package config

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadControlReadsExplicitConfig(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("12345678901234567890123456789012"))
	workOSCookiePassword := "12345678901234567890123456789012"

	t.Setenv("ENVIRONMENT", "production")
	t.Setenv("NATS_URL", " nats://10.0.0.1:4222 ")
	t.Setenv("DATABASE_URL", " postgres://user:pass@db/spacescale ")
	t.Setenv("LISTEN_ADDR", " 127.0.0.1:9090 ")
	t.Setenv("AUTH_ENABLED", "true")
	t.Setenv("AUTH_JWT_SECRET", " secret ")
	t.Setenv("AUTH_ISSUER", " issuer ")
	t.Setenv("AUTH_AUDIENCE", " audience ")
	t.Setenv("API_ENV_ENCRYPTION_KEY_ID", " key-v1 ")
	t.Setenv("API_ENV_ENCRYPTION_KEY", key)
	t.Setenv("WORKOS_API_KEY", " workos-key ")
	t.Setenv("WORKOS_CLIENT_ID", " workos-client ")
	t.Setenv("WORKOS_COOKIE_PASSWORD", workOSCookiePassword)
	t.Setenv("WORKOS_REDIRECT_URI", " https://example.com/workos/callback ")
	t.Setenv("WORKOS_POST_LOGIN_REDIRECT_URI", " https://example.com/app ")
	t.Setenv("WORKOS_LOGOUT_REDIRECT_URI", " https://example.com/logout ")

	cfg, err := LoadControl()
	require.NoError(t, err)

	assert.Equal(t, productionEnvironment, cfg.Environment)
	assert.Equal(t, "nats://10.0.0.1:4222", cfg.NATSURL)
	assert.Equal(t, "postgres://user:pass@db/spacescale", cfg.DatabaseURL)
	assert.Equal(t, "127.0.0.1:9090", cfg.ListenAddr)
	assert.True(t, cfg.Auth.Enabled)
	assert.Equal(t, "secret", cfg.Auth.JWTSecret)
	assert.Equal(t, "issuer", cfg.Auth.Issuer)
	assert.Equal(t, "audience", cfg.Auth.Audience)
	assert.Equal(t, "key-v1", cfg.EnvEncryptionKeyID)
	assert.Len(t, cfg.EnvEncryptionKey, 32)
	assert.Equal(t, "workos-key", cfg.WorkOS.APIKey)
	assert.Equal(t, "workos-client", cfg.WorkOS.ClientID)
	assert.Equal(t, workOSCookiePassword, cfg.WorkOS.CookiePassword)
	assert.Equal(t, "https://example.com/workos/callback", cfg.WorkOS.RedirectURI)
	assert.Equal(t, "https://example.com/app", cfg.WorkOS.PostLoginRedirectURI)
	assert.Equal(t, "https://example.com/logout", cfg.WorkOS.LogoutRedirectURI)
	assert.Equal(t, defaultWorkOSCookieName, cfg.WorkOS.CookieName)
}

func TestLoadControlAcceptsExplicitDevelopmentEnvironment(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("12345678901234567890123456789012"))

	t.Setenv("ENVIRONMENT", developmentEnvironment)
	t.Setenv("DATABASE_URL", "postgres://db")
	t.Setenv("API_ENV_ENCRYPTION_KEY_ID", "key-v1")
	t.Setenv("API_ENV_ENCRYPTION_KEY", key)

	cfg, err := LoadControl()
	require.NoError(t, err)
	assert.Equal(t, developmentEnvironment, cfg.Environment)
}

func TestLoadControlAppliesDefaults(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("12345678901234567890123456789012"))

	t.Setenv("DATABASE_URL", "postgres://db")
	t.Setenv("API_ENV_ENCRYPTION_KEY_ID", "key-v1")
	t.Setenv("API_ENV_ENCRYPTION_KEY", key)

	cfg, err := LoadControl()
	require.NoError(t, err)

	assert.Equal(t, developmentEnvironment, cfg.Environment)
	assert.Equal(t, defaultNATSURL, cfg.NATSURL)
	assert.Equal(t, defaultListenAddr, cfg.ListenAddr)
	assert.False(t, cfg.Auth.Enabled)
	assert.Equal(t, defaultAuthIssuer, cfg.Auth.Issuer)
	assert.Equal(t, defaultAuthAudience, cfg.Auth.Audience)
	assert.Empty(t, cfg.Auth.JWTSecret)
	assert.Equal(t, WorkOSConfig{CookieName: defaultWorkOSCookieName}, cfg.WorkOS)
}

func TestLoadControlAppliesDevelopmentWorkOSDefaults(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("12345678901234567890123456789012"))
	workOSCookiePassword := "12345678901234567890123456789012"

	t.Setenv("DATABASE_URL", "postgres://db")
	t.Setenv("API_ENV_ENCRYPTION_KEY_ID", "key-v1")
	t.Setenv("API_ENV_ENCRYPTION_KEY", key)
	t.Setenv("WORKOS_API_KEY", "workos-key")
	t.Setenv("WORKOS_CLIENT_ID", "workos-client")
	t.Setenv("WORKOS_COOKIE_PASSWORD", workOSCookiePassword)

	cfg, err := LoadControl()
	require.NoError(t, err)

	assert.Equal(t, "workos-key", cfg.WorkOS.APIKey)
	assert.Equal(t, "workos-client", cfg.WorkOS.ClientID)
	assert.Equal(t, workOSCookiePassword, cfg.WorkOS.CookiePassword)
	assert.Equal(t, defaultWorkOSRedirectURI, cfg.WorkOS.RedirectURI)
	assert.Equal(t, defaultWorkOSPostLoginRedirectURI, cfg.WorkOS.PostLoginRedirectURI)
	assert.Equal(t, defaultWorkOSLogoutRedirectURI, cfg.WorkOS.LogoutRedirectURI)
	assert.Equal(t, defaultWorkOSCookieName, cfg.WorkOS.CookieName)
}

func TestLoadControlRejectsMissingRequiredConfig(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("12345678901234567890123456789012"))

	tests := []struct {
		name       string
		setenv     func(*testing.T)
		wantErrMsg string
	}{
		{
			name: "invalid environment",
			setenv: func(t *testing.T) {
				t.Helper()
				t.Setenv("ENVIRONMENT", "staging")
				t.Setenv("DATABASE_URL", "postgres://db")
				t.Setenv("API_ENV_ENCRYPTION_KEY_ID", "key-v1")
				t.Setenv("API_ENV_ENCRYPTION_KEY", key)
			},
			wantErrMsg: "invalid config ENVIRONMENT",
		},
		{
			name: "missing database url",
			setenv: func(t *testing.T) {
				t.Helper()
				t.Setenv("API_ENV_ENCRYPTION_KEY_ID", "key-v1")
				t.Setenv("API_ENV_ENCRYPTION_KEY", key)
			},
			wantErrMsg: "missing required config DATABASE_URL",
		},
		{
			name: "missing jwt secret when auth enabled",
			setenv: func(t *testing.T) {
				t.Helper()
				t.Setenv("DATABASE_URL", "postgres://db")
				t.Setenv("AUTH_ENABLED", "true")
				t.Setenv("API_ENV_ENCRYPTION_KEY_ID", "key-v1")
				t.Setenv("API_ENV_ENCRYPTION_KEY", key)
			},
			wantErrMsg: "missing required config AUTH_JWT_SECRET",
		},
		{
			name: "missing workos api key when workos is enabled",
			setenv: func(t *testing.T) {
				t.Helper()
				t.Setenv("DATABASE_URL", "postgres://db")
				t.Setenv("API_ENV_ENCRYPTION_KEY_ID", "key-v1")
				t.Setenv("API_ENV_ENCRYPTION_KEY", key)
				t.Setenv("WORKOS_CLIENT_ID", "workos-client")
				t.Setenv("WORKOS_COOKIE_PASSWORD", "12345678901234567890123456789012")
			},
			wantErrMsg: "missing required config WORKOS_API_KEY",
		},
		{
			name: "missing workos client id when workos is enabled",
			setenv: func(t *testing.T) {
				t.Helper()
				t.Setenv("DATABASE_URL", "postgres://db")
				t.Setenv("API_ENV_ENCRYPTION_KEY_ID", "key-v1")
				t.Setenv("API_ENV_ENCRYPTION_KEY", key)
				t.Setenv("WORKOS_API_KEY", "workos-key")
				t.Setenv("WORKOS_COOKIE_PASSWORD", "12345678901234567890123456789012")
			},
			wantErrMsg: "missing required config WORKOS_CLIENT_ID",
		},
		{
			name: "missing workos cookie password when workos is enabled",
			setenv: func(t *testing.T) {
				t.Helper()
				t.Setenv("DATABASE_URL", "postgres://db")
				t.Setenv("API_ENV_ENCRYPTION_KEY_ID", "key-v1")
				t.Setenv("API_ENV_ENCRYPTION_KEY", key)
				t.Setenv("WORKOS_API_KEY", "workos-key")
				t.Setenv("WORKOS_CLIENT_ID", "workos-client")
			},
			wantErrMsg: "missing required config WORKOS_COOKIE_PASSWORD",
		},
		{
			name: "missing workos redirect uri in production",
			setenv: func(t *testing.T) {
				t.Helper()
				t.Setenv("ENVIRONMENT", "production")
				t.Setenv("DATABASE_URL", "postgres://db")
				t.Setenv("API_ENV_ENCRYPTION_KEY_ID", "key-v1")
				t.Setenv("API_ENV_ENCRYPTION_KEY", key)
				t.Setenv("WORKOS_API_KEY", "workos-key")
				t.Setenv("WORKOS_CLIENT_ID", "workos-client")
				t.Setenv("WORKOS_COOKIE_PASSWORD", "12345678901234567890123456789012")
			},
			wantErrMsg: "missing required config WORKOS_REDIRECT_URI",
		},
		{
			name: "missing workos post login redirect uri in production",
			setenv: func(t *testing.T) {
				t.Helper()
				t.Setenv("ENVIRONMENT", "production")
				t.Setenv("DATABASE_URL", "postgres://db")
				t.Setenv("API_ENV_ENCRYPTION_KEY_ID", "key-v1")
				t.Setenv("API_ENV_ENCRYPTION_KEY", key)
				t.Setenv("WORKOS_API_KEY", "workos-key")
				t.Setenv("WORKOS_CLIENT_ID", "workos-client")
				t.Setenv("WORKOS_COOKIE_PASSWORD", "12345678901234567890123456789012")
				t.Setenv("WORKOS_REDIRECT_URI", "https://example.com/workos/callback")
			},
			wantErrMsg: "missing required config WORKOS_POST_LOGIN_REDIRECT_URI",
		},
		{
			name: "missing workos logout redirect uri in production",
			setenv: func(t *testing.T) {
				t.Helper()
				t.Setenv("ENVIRONMENT", "production")
				t.Setenv("DATABASE_URL", "postgres://db")
				t.Setenv("API_ENV_ENCRYPTION_KEY_ID", "key-v1")
				t.Setenv("API_ENV_ENCRYPTION_KEY", key)
				t.Setenv("WORKOS_API_KEY", "workos-key")
				t.Setenv("WORKOS_CLIENT_ID", "workos-client")
				t.Setenv("WORKOS_COOKIE_PASSWORD", "12345678901234567890123456789012")
				t.Setenv("WORKOS_REDIRECT_URI", "https://example.com/workos/callback")
				t.Setenv("WORKOS_POST_LOGIN_REDIRECT_URI", "https://example.com/app")
			},
			wantErrMsg: "missing required config WORKOS_LOGOUT_REDIRECT_URI",
		},
		{
			name: "missing key id",
			setenv: func(t *testing.T) {
				t.Helper()
				t.Setenv("DATABASE_URL", "postgres://db")
				t.Setenv("API_ENV_ENCRYPTION_KEY", key)
			},
			wantErrMsg: "missing required config API_ENV_ENCRYPTION_KEY_ID",
		},
		{
			name: "missing key",
			setenv: func(t *testing.T) {
				t.Helper()
				t.Setenv("DATABASE_URL", "postgres://db")
				t.Setenv("API_ENV_ENCRYPTION_KEY_ID", "key-v1")
			},
			wantErrMsg: "missing required config API_ENV_ENCRYPTION_KEY",
		},
		{
			name: "short workos cookie password",
			setenv: func(t *testing.T) {
				t.Helper()
				t.Setenv("DATABASE_URL", "postgres://db")
				t.Setenv("API_ENV_ENCRYPTION_KEY_ID", "key-v1")
				t.Setenv("API_ENV_ENCRYPTION_KEY", key)
				t.Setenv("WORKOS_API_KEY", "workos-key")
				t.Setenv("WORKOS_CLIENT_ID", "workos-client")
				t.Setenv("WORKOS_COOKIE_PASSWORD", "too-short")
				t.Setenv("WORKOS_REDIRECT_URI", "https://example.com/workos/callback")
				t.Setenv("WORKOS_POST_LOGIN_REDIRECT_URI", "https://example.com/app")
				t.Setenv("WORKOS_LOGOUT_REDIRECT_URI", "https://example.com/logout")
			},
			wantErrMsg: "invalid config WORKOS_COOKIE_PASSWORD: must be at least 32 characters",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.setenv(t)
			_, err := LoadControl()
			require.EqualError(t, err, tc.wantErrMsg)
		})
	}
}

func TestLoadScaledReadsExplicitConfig(t *testing.T) {
	t.Setenv("ENVIRONMENT", "production")
	t.Setenv("NATS_URL", " nats://10.0.0.1:4222 ")

	cfg, err := LoadScaled()
	require.NoError(t, err)
	assert.Equal(t, productionEnvironment, cfg.Environment)
	assert.Equal(t, "nats://10.0.0.1:4222", cfg.NATSURL)
}

func TestLoadScaledAcceptsExplicitDevelopmentEnvironment(t *testing.T) {
	t.Setenv("ENVIRONMENT", developmentEnvironment)

	cfg, err := LoadScaled()
	require.NoError(t, err)
	assert.Equal(t, developmentEnvironment, cfg.Environment)
}

func TestLoadScaledRejectsInvalidEnvironment(t *testing.T) {
	t.Setenv("ENVIRONMENT", "staging")

	_, err := LoadScaled()
	require.EqualError(t, err, "invalid config ENVIRONMENT")
}

func TestLoadScaledAppliesDefaults(t *testing.T) {
	cfg, err := LoadScaled()
	require.NoError(t, err)
	assert.Equal(t, developmentEnvironment, cfg.Environment)
	assert.Equal(t, defaultNATSURL, cfg.NATSURL)
}

func TestDecodeEnvEncryptionKey(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		wantErrMsg  string
		wantKeySize int
	}{
		{
			name:        "missing key",
			raw:         "",
			wantErrMsg:  "missing required config API_ENV_ENCRYPTION_KEY",
			wantKeySize: 0,
		},
		{
			name:        "invalid base64",
			raw:         "%%%",
			wantErrMsg:  "invalid config API_ENV_ENCRYPTION_KEY: must be base64-encoded 32-byte key",
			wantKeySize: 0,
		},
		{
			name:        "wrong decoded size",
			raw:         base64.StdEncoding.EncodeToString([]byte("short")),
			wantErrMsg:  "invalid config API_ENV_ENCRYPTION_KEY: must decode to 32 bytes",
			wantKeySize: 0,
		},
		{
			name:        "valid key",
			raw:         base64.StdEncoding.EncodeToString([]byte("12345678901234567890123456789012")),
			wantErrMsg:  "",
			wantKeySize: 32,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			key, err := decodeEnvEncryptionKey(tc.raw, "API_ENV_ENCRYPTION_KEY")
			if tc.wantErrMsg == "" {
				require.NoError(t, err)
				assert.Len(t, key, tc.wantKeySize)
				return
			}

			require.EqualError(t, err, tc.wantErrMsg)
			assert.Nil(t, key)
		})
	}
}
