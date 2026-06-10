package config

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadControlReadsExplicitConfig(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("12345678901234567890123456789012"))

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
