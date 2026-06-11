package config

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setValidControlEnv(t *testing.T, key string) {
	t.Helper()
	t.Setenv("ENVIRONMENT", "development")
	t.Setenv("NATS_URL", "nats://127.0.0.1:4222")
	t.Setenv("DATABASE_URL", "postgres://db")
	t.Setenv("API_ENV_ENCRYPTION_KEY_ID", "key-v1")
	t.Setenv("API_ENV_ENCRYPTION_KEY", key)
	t.Setenv("WORKOS_API_KEY", "workos-key")
	t.Setenv("WORKOS_CLIENT_ID", "workos-client")
	t.Setenv("WORKOS_COOKIE_PASSWORD", "12345678901234567890123456789012")
	t.Setenv("WORKOS_REDIRECT_URI", "https://example.com/workos/callback")
	t.Setenv("WORKOS_POST_LOGIN_REDIRECT_URI", "https://example.com/app")
	t.Setenv("WORKOS_LOGOUT_REDIRECT_URI", "https://example.com/logout")
}

func TestLoadControlReadsExplicitConfig(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("12345678901234567890123456789012"))

	t.Setenv("ENVIRONMENT", "production")
	t.Setenv("NATS_URL", "nats://10.0.0.1:4222")
	t.Setenv("DATABASE_URL", "postgres://user:pass@db/spacescale")
	t.Setenv("API_ENV_ENCRYPTION_KEY_ID", " key-v1 ")
	t.Setenv("API_ENV_ENCRYPTION_KEY", key)
	t.Setenv("WORKOS_API_KEY", "workos-key")
	t.Setenv("WORKOS_CLIENT_ID", "workos-client")
	t.Setenv("WORKOS_COOKIE_PASSWORD", "12345678901234567890123456789012")
	t.Setenv("WORKOS_REDIRECT_URI", "https://example.com/workos/callback")
	t.Setenv("WORKOS_POST_LOGIN_REDIRECT_URI", "https://example.com/app")
	t.Setenv("WORKOS_LOGOUT_REDIRECT_URI", "https://example.com/logout")

	cfg, err := LoadControl()
	require.NoError(t, err)

	assert.Equal(t, "production", cfg.Environment)
	assert.Equal(t, "nats://10.0.0.1:4222", cfg.NATSURL)
	assert.Equal(t, "postgres://user:pass@db/spacescale", cfg.DatabaseURL)
	assert.Equal(t, defaultListenAddr, cfg.ListenAddr)
	assert.Equal(t, "key-v1", cfg.EnvEncryptionKeyID)
	assert.Len(t, cfg.EnvEncryptionKey, 32)
	assert.Equal(t, "workos-key", cfg.WorkOS.APIKey)
	assert.Equal(t, "workos-client", cfg.WorkOS.ClientID)
	assert.Equal(t, "12345678901234567890123456789012", cfg.WorkOS.CookiePassword)
	assert.Equal(t, "https://example.com/workos/callback", cfg.WorkOS.RedirectURI)
	assert.Equal(t, "https://example.com/app", cfg.WorkOS.PostLoginRedirectURI)
	assert.Equal(t, "https://example.com/logout", cfg.WorkOS.LogoutRedirectURI)
	assert.Equal(t, workOSCookieName, cfg.WorkOS.CookieName)
}

func TestLoadControlReadsTrimmedConfig(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("12345678901234567890123456789012"))

	t.Setenv("ENVIRONMENT", "development")
	t.Setenv("NATS_URL", " nats://127.0.0.1:4222 ")
	t.Setenv("DATABASE_URL", " postgres://db ")
	t.Setenv("API_ENV_ENCRYPTION_KEY_ID", " key-v1 ")
	t.Setenv("API_ENV_ENCRYPTION_KEY", key)
	t.Setenv("WORKOS_API_KEY", " workos-key ")
	t.Setenv("WORKOS_CLIENT_ID", " workos-client ")
	t.Setenv("WORKOS_COOKIE_PASSWORD", "12345678901234567890123456789012")
	t.Setenv("WORKOS_REDIRECT_URI", " https://example.com/workos/callback ")
	t.Setenv("WORKOS_POST_LOGIN_REDIRECT_URI", " https://example.com/app ")
	t.Setenv("WORKOS_LOGOUT_REDIRECT_URI", " https://example.com/logout ")

	cfg, err := LoadControl()
	require.NoError(t, err)

	assert.Equal(t, "development", cfg.Environment)
	assert.Equal(t, "nats://127.0.0.1:4222", cfg.NATSURL)
	assert.Equal(t, "postgres://db", cfg.DatabaseURL)
	assert.Equal(t, defaultListenAddr, cfg.ListenAddr)
	assert.Equal(t, "key-v1", cfg.EnvEncryptionKeyID)
	assert.Equal(t, "workos-key", cfg.WorkOS.APIKey)
	assert.Equal(t, "workos-client", cfg.WorkOS.ClientID)
	assert.Equal(t, "12345678901234567890123456789012", cfg.WorkOS.CookiePassword)
	assert.Equal(t, "https://example.com/workos/callback", cfg.WorkOS.RedirectURI)
	assert.Equal(t, "https://example.com/app", cfg.WorkOS.PostLoginRedirectURI)
	assert.Equal(t, "https://example.com/logout", cfg.WorkOS.LogoutRedirectURI)
	assert.Equal(t, workOSCookieName, cfg.WorkOS.CookieName)
}

func TestLoadControlRejectsMissingRequiredConfig(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("12345678901234567890123456789012"))

	tests := []struct {
		name       string
		setenv     func(*testing.T)
		wantErrMsg string
	}{
		{
			name: "missing environment",
			setenv: func(t *testing.T) {
				t.Helper()
				t.Setenv("NATS_URL", "nats://127.0.0.1:4222")
				t.Setenv("DATABASE_URL", "postgres://db")
				t.Setenv("API_ENV_ENCRYPTION_KEY_ID", "key-v1")
				t.Setenv("API_ENV_ENCRYPTION_KEY", key)
				setValidControlEnv(t, key)
				// unset environment last so the required tag sees it missing.
				t.Setenv("ENVIRONMENT", "")
			},
			wantErrMsg: "Key: 'Control.Environment' Error:Field validation for 'Environment' failed on the 'required' tag",
		},
		{
			name: "invalid environment",
			setenv: func(t *testing.T) {
				t.Helper()
				setValidControlEnv(t, key)
				t.Setenv("ENVIRONMENT", "staging")
			},
			wantErrMsg: "Key: 'Control.Environment' Error:Field validation for 'Environment' failed on the 'oneof' tag",
		},
		{
			name: "missing database url",
			setenv: func(t *testing.T) {
				t.Helper()
				t.Setenv("ENVIRONMENT", "development")
				t.Setenv("NATS_URL", "nats://127.0.0.1:4222")
				t.Setenv("API_ENV_ENCRYPTION_KEY_ID", "key-v1")
				t.Setenv("API_ENV_ENCRYPTION_KEY", key)
				setValidControlEnv(t, key)
				t.Setenv("DATABASE_URL", "")
			},
			wantErrMsg: "Key: 'Control.DatabaseURL' Error:Field validation for 'DatabaseURL' failed on the 'required' tag",
		},
		{
			name: "missing workos api key",
			setenv: func(t *testing.T) {
				t.Helper()
				setValidControlEnv(t, key)
				t.Setenv("WORKOS_API_KEY", "")
			},
			wantErrMsg: "Key: 'Control.WorkOS.APIKey' Error:Field validation for 'APIKey' failed on the 'required' tag",
		},
		{
			name: "missing workos client id",
			setenv: func(t *testing.T) {
				t.Helper()
				setValidControlEnv(t, key)
				t.Setenv("WORKOS_CLIENT_ID", "")
			},
			wantErrMsg: "Key: 'Control.WorkOS.ClientID' Error:Field validation for 'ClientID' failed on the 'required' tag",
		},
		{
			name: "missing workos cookie password",
			setenv: func(t *testing.T) {
				t.Helper()
				setValidControlEnv(t, key)
				t.Setenv("WORKOS_COOKIE_PASSWORD", "")
			},
			wantErrMsg: "Key: 'Control.WorkOS.CookiePassword' Error:Field validation for 'CookiePassword' failed on the 'required' tag",
		},
		{
			name: "short workos cookie password",
			setenv: func(t *testing.T) {
				t.Helper()
				setValidControlEnv(t, key)
				t.Setenv("WORKOS_COOKIE_PASSWORD", "too-short")
			},
			wantErrMsg: "Key: 'Control.WorkOS.CookiePassword' Error:Field validation for 'CookiePassword' failed on the 'min' tag",
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
	assert.Equal(t, "production", cfg.Environment)
	assert.Equal(t, "nats://10.0.0.1:4222", cfg.NATSURL)
}

func TestLoadScaledRejectsMissingEnvironment(t *testing.T) {
	t.Setenv("NATS_URL", "nats://127.0.0.1:4222")
	_, err := LoadScaled()
	require.EqualError(t, err, "Key: 'Scaled.Environment' Error:Field validation for 'Environment' failed on the 'required' tag")
}

func TestLoadScaledRejectsInvalidEnvironment(t *testing.T) {
	t.Setenv("ENVIRONMENT", "staging")
	t.Setenv("NATS_URL", "nats://127.0.0.1:4222")

	_, err := LoadScaled()
	require.EqualError(t, err, "Key: 'Scaled.Environment' Error:Field validation for 'Environment' failed on the 'oneof' tag")
}

func TestDecodeEnvEncryptionKey(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		wantErrMsg  string
		wantKeySize int
	}{
		{name: "invalid base64", raw: "%%%", wantErrMsg: "invalid config API_ENV_ENCRYPTION_KEY: must be base64-encoded 32-byte key"},
		{name: "wrong decoded size", raw: base64.StdEncoding.EncodeToString([]byte("short")), wantErrMsg: "invalid config API_ENV_ENCRYPTION_KEY: must decode to 32 bytes"},
		{name: "valid key", raw: base64.StdEncoding.EncodeToString([]byte("12345678901234567890123456789012")), wantKeySize: 32},
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
