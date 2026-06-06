package config

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadControl(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("12345678901234567890123456789012"))

	t.Setenv("APP_ENV", " prod ")
	t.Setenv("NATS_URL", " nats://10.0.0.1:4222 ")
	t.Setenv("DATABASE_URL", " postgres://user:pass@db/spacescale ")
	t.Setenv("PORT", " 9090 ")
	t.Setenv("BFF_JWT_SECRET", " secret ")
	t.Setenv("BFF_JWT_ISSUER", " issuer ")
	t.Setenv("BFF_JWT_AUDIENCE", " audience ")
	t.Setenv("INTERNAL_AUTH_SYNC_SECRET", " internal-secret ")
	t.Setenv("API_ENV_ENCRYPTION_KEY_ID", " key-v1 ")
	t.Setenv("API_ENV_ENCRYPTION_KEY", key)

	cfg, err := LoadControl()
	require.NoError(t, err)

	assert.Equal(t, "production", cfg.Environment)
	assert.Equal(t, "nats://10.0.0.1:4222", cfg.NATSURL)
	assert.Equal(t, "postgres://user:pass@db/spacescale", cfg.DatabaseURL)
	assert.Equal(t, "9090", cfg.Port)
	assert.Equal(t, "secret", cfg.Auth.JWTSecret)
	assert.Equal(t, "issuer", cfg.Auth.Issuer)
	assert.Equal(t, "audience", cfg.Auth.Audience)
	assert.Equal(t, "internal-secret", cfg.InternalAuthSecret)
	assert.Equal(t, "key-v1", cfg.EnvEncryptionKeyID)
	assert.Len(t, cfg.EnvEncryptionKey, 32)
}

func TestLoadControlAppliesDefaults(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("12345678901234567890123456789012"))

	t.Setenv("DATABASE_URL", "postgres://db")
	t.Setenv("BFF_JWT_SECRET", "secret")
	t.Setenv("INTERNAL_AUTH_SYNC_SECRET", "internal-secret")
	t.Setenv("API_ENV_ENCRYPTION_KEY_ID", "key-v1")
	t.Setenv("API_ENV_ENCRYPTION_KEY", key)

	cfg, err := LoadControl()
	require.NoError(t, err)

	assert.Equal(t, defaultEnvironment, cfg.Environment)
	assert.Equal(t, defaultNATSURL, cfg.NATSURL)
	assert.Equal(t, defaultPort, cfg.Port)
	assert.Equal(t, defaultAuthIssuer, cfg.Auth.Issuer)
	assert.Equal(t, defaultAuthAudience, cfg.Auth.Audience)
}

func TestLoadControlRejectsMissingRequiredConfig(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("12345678901234567890123456789012"))

	tests := []struct {
		name       string
		setenv     func(*testing.T)
		wantErrMsg string
	}{
		{
			name: "missing database url",
			setenv: func(t *testing.T) {
				t.Setenv("BFF_JWT_SECRET", "secret")
				t.Setenv("INTERNAL_AUTH_SYNC_SECRET", "internal-secret")
				t.Setenv("API_ENV_ENCRYPTION_KEY_ID", "key-v1")
				t.Setenv("API_ENV_ENCRYPTION_KEY", key)
			},
			wantErrMsg: "missing required config DATABASE_URL",
		},
		{
			name: "missing jwt secret",
			setenv: func(t *testing.T) {
				t.Setenv("DATABASE_URL", "postgres://db")
				t.Setenv("INTERNAL_AUTH_SYNC_SECRET", "internal-secret")
				t.Setenv("API_ENV_ENCRYPTION_KEY_ID", "key-v1")
				t.Setenv("API_ENV_ENCRYPTION_KEY", key)
			},
			wantErrMsg: "missing required config BFF_JWT_SECRET",
		},
		{
			name: "missing internal auth secret",
			setenv: func(t *testing.T) {
				t.Setenv("DATABASE_URL", "postgres://db")
				t.Setenv("BFF_JWT_SECRET", "secret")
				t.Setenv("API_ENV_ENCRYPTION_KEY_ID", "key-v1")
				t.Setenv("API_ENV_ENCRYPTION_KEY", key)
			},
			wantErrMsg: "missing required config INTERNAL_AUTH_SYNC_SECRET",
		},
		{
			name: "missing key id",
			setenv: func(t *testing.T) {
				t.Setenv("DATABASE_URL", "postgres://db")
				t.Setenv("BFF_JWT_SECRET", "secret")
				t.Setenv("INTERNAL_AUTH_SYNC_SECRET", "internal-secret")
				t.Setenv("API_ENV_ENCRYPTION_KEY", key)
			},
			wantErrMsg: "missing required config API_ENV_ENCRYPTION_KEY_ID",
		},
		{
			name: "missing key",
			setenv: func(t *testing.T) {
				t.Setenv("DATABASE_URL", "postgres://db")
				t.Setenv("BFF_JWT_SECRET", "secret")
				t.Setenv("INTERNAL_AUTH_SYNC_SECRET", "internal-secret")
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

func TestLoadScaled(t *testing.T) {
	t.Setenv("ENVIRONMENT", " prod ")
	t.Setenv("NATS_URL", " nats://10.0.0.1:4222 ")

	cfg := LoadScaled()
	assert.Equal(t, "production", cfg.Environment)
	assert.Equal(t, "nats://10.0.0.1:4222", cfg.NATSURL)
}

func TestLoadScaledAppliesDefaults(t *testing.T) {
	cfg := LoadScaled()
	assert.Equal(t, defaultEnvironment, cfg.Environment)
	assert.Equal(t, defaultNATSURL, cfg.NATSURL)
}

func TestControlListenAddr(t *testing.T) {
	tests := []struct {
		name string
		cfg  Control
		want string
	}{
		{name: "bare port", cfg: Control{Port: "8081"}, want: ":8081"},
		{name: "host and port", cfg: Control{Port: "127.0.0.1:8081"}, want: "127.0.0.1:8081"},
		{name: "default port", cfg: Control{}, want: ":8080"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.cfg.ListenAddr())
		})
	}
}

func TestDecodeEnvEncryptionKey(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		wantErrMsg  string
		wantKeySize int
	}{
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
