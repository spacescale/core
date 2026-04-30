// Copyright (c) 2026 SpaceScale Systems Inc. All rights reserved.

package config

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("12345678901234567890123456789012"))

	t.Setenv("APP_ENV", " prod ")
	t.Setenv("NATS_URL", " nats://10.0.0.1:4222 ")
	t.Setenv("DATABASE_URL", " postgres://user:pass@db/spacescale ")
	t.Setenv("PORT", " 9090 ")
	t.Setenv("FIRECRACKER_BIN", " /opt/firecracker ")
	t.Setenv("BFF_JWT_SECRET", " secret ")
	t.Setenv("BFF_JWT_ISSUER", " issuer ")
	t.Setenv("BFF_JWT_AUDIENCE", " audience ")
	t.Setenv("INTERNAL_AUTH_SYNC_SECRET", " internal-secret ")
	t.Setenv("API_ENV_ENCRYPTION_KEY_ID", " key-v1 ")
	t.Setenv("API_ENV_ENCRYPTION_KEY", key)

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, "production", cfg.Environment)
	assert.Equal(t, "nats://10.0.0.1:4222", cfg.NATSURL)
	assert.Equal(t, "postgres://user:pass@db/spacescale", cfg.DatabaseURL)
	assert.Equal(t, "9090", cfg.Port)
	assert.Equal(t, "/opt/firecracker", cfg.FirecrackerBin)
	assert.Equal(t, "secret", cfg.Auth.JWTSecret)
	assert.Equal(t, "issuer", cfg.Auth.Issuer)
	assert.Equal(t, "audience", cfg.Auth.Audience)
	assert.Equal(t, "internal-secret", cfg.InternalAuthSecret)
	assert.Equal(t, "key-v1", cfg.EnvEncryptionKeyID)
	assert.Len(t, cfg.EnvEncryptionKey, 32)
}

func TestConfigNormalized(t *testing.T) {
	cfg := Config{
		Environment:        " prod ",
		NATSURL:            "   ",
		DatabaseURL:        " postgres://db ",
		Port:               "   ",
		FirecrackerBin:     "   ",
		InternalAuthSecret: " secret ",
		EnvEncryptionKeyID: " key-id ",
		Auth: AuthConfig{
			JWTSecret: " jwt-secret ",
			Issuer:    "   ",
			Audience:  "   ",
		},
	}

	normalized := cfg.Normalized()

	assert.Equal(t, "production", normalized.Environment)
	assert.Equal(t, defaultNATSURL, normalized.NATSURL)
	assert.Equal(t, "postgres://db", normalized.DatabaseURL)
	assert.Equal(t, defaultPort, normalized.Port)
	assert.Equal(t, defaultFirecrackerBin, normalized.FirecrackerBin)
	assert.Equal(t, "secret", normalized.InternalAuthSecret)
	assert.Equal(t, "key-id", normalized.EnvEncryptionKeyID)
	assert.Equal(t, "jwt-secret", normalized.Auth.JWTSecret)
	assert.Equal(t, defaultAuthIssuer, normalized.Auth.Issuer)
	assert.Equal(t, defaultAuthAudience, normalized.Auth.Audience)
}

func TestConfigListenAddr(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{name: "bare port", cfg: Config{Port: "8081"}, want: ":8081"},
		{name: "host and port", cfg: Config{Port: "127.0.0.1:8081"}, want: "127.0.0.1:8081"},
		{name: "default port", cfg: Config{}, want: ":8080"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.cfg.ListenAddr())
		})
	}
}

func TestConfigValidateScalecp(t *testing.T) {
	valid := Config{
		DatabaseURL:        "postgres://db",
		InternalAuthSecret: "internal-secret",
		EnvEncryptionKeyID: "key-v1",
		EnvEncryptionKey:   []byte("12345678901234567890123456789012"),
		Auth: AuthConfig{
			JWTSecret: "jwt-secret",
			Issuer:    "issuer",
			Audience:  "audience",
		},
	}

	tests := []struct {
		name       string
		cfg        Config
		wantErrMsg string
	}{
		{name: "valid config", cfg: valid},
		{name: "missing database url", cfg: Config{Auth: valid.Auth, InternalAuthSecret: valid.InternalAuthSecret, EnvEncryptionKeyID: valid.EnvEncryptionKeyID, EnvEncryptionKey: valid.EnvEncryptionKey}, wantErrMsg: "missing required config DATABASE_URL"},
		{name: "missing jwt secret", cfg: Config{DatabaseURL: valid.DatabaseURL, InternalAuthSecret: valid.InternalAuthSecret, EnvEncryptionKeyID: valid.EnvEncryptionKeyID, EnvEncryptionKey: valid.EnvEncryptionKey, Auth: AuthConfig{Issuer: "issuer", Audience: "audience"}}, wantErrMsg: "missing required config BFF_JWT_SECRET"},
		{name: "missing internal auth secret", cfg: Config{DatabaseURL: valid.DatabaseURL, Auth: valid.Auth, EnvEncryptionKeyID: valid.EnvEncryptionKeyID, EnvEncryptionKey: valid.EnvEncryptionKey}, wantErrMsg: "missing required config INTERNAL_AUTH_SYNC_SECRET"},
		{name: "missing key id", cfg: Config{DatabaseURL: valid.DatabaseURL, Auth: valid.Auth, InternalAuthSecret: valid.InternalAuthSecret, EnvEncryptionKey: valid.EnvEncryptionKey}, wantErrMsg: "missing required config API_ENV_ENCRYPTION_KEY_ID"},
		{name: "missing key", cfg: Config{DatabaseURL: valid.DatabaseURL, Auth: valid.Auth, InternalAuthSecret: valid.InternalAuthSecret, EnvEncryptionKeyID: valid.EnvEncryptionKeyID}, wantErrMsg: "missing required config API_ENV_ENCRYPTION_KEY"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.cfg.ValidateScalecp()
			if tc.wantErrMsg == "" {
				require.NoError(t, err)
				return
			}
			require.EqualError(t, err, tc.wantErrMsg)
		})
	}
}

func TestConfigValidateScaled(t *testing.T) {
	_, err := (Config{}).ValidateScaled()
	require.NoError(t, err)
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
		tc := tc
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

func TestValidateEnvEncryptionKeyID(t *testing.T) {
	tests := []struct {
		name       string
		keyID      string
		wantErrMsg string
	}{
		{name: "valid key id", keyID: "key-v1"},
		{name: "contains colon", keyID: "key:v1", wantErrMsg: "invalid config API_ENV_ENCRYPTION_KEY_ID: key id must not contain ':' or whitespace"},
		{name: "contains whitespace", keyID: "key id", wantErrMsg: "invalid config API_ENV_ENCRYPTION_KEY_ID: key id must not contain ':' or whitespace"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := validateEnvEncryptionKeyID(tc.keyID, "API_ENV_ENCRYPTION_KEY_ID")
			if tc.wantErrMsg == "" {
				require.NoError(t, err)
				return
			}
			require.EqualError(t, err, tc.wantErrMsg)
		})
	}
}
