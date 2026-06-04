package auth

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/spacescale/core/internal/shared/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testJWTSecret   = "test-bff-secret"
	testJWTIssuer   = "spacescale-web-bff"
	testJWTAudience = "spacescale-api"
	testIdentityKey = "t0gun"
)

type testJWTClaims struct {
	IdentityKeyClaim string `json:"identity_key,omitempty"`
	jwt.RegisteredClaims
}

func defaultAuthCfg() config.AuthConfig {
	return config.AuthConfig{
		JWTSecret: testJWTSecret,
		Issuer:    testJWTIssuer,
		Audience:  testJWTAudience,
	}
}

func validClaims(now time.Time) testJWTClaims {
	return testJWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "github:" + testIdentityKey,
			Issuer:    testJWTIssuer,
			Audience:  jwt.ClaimStrings{testJWTAudience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(10 * time.Minute)),
		},
	}
}

func mintToken(t *testing.T, secret string, claims testJWTClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	raw, err := token.SignedString([]byte(secret))
	require.NoError(t, err)
	return raw
}

func mintTokenWithMethod(t *testing.T, method jwt.SigningMethod, secret string, claims testJWTClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(method, claims)
	raw, err := token.SignedString([]byte(secret))
	require.NoError(t, err)
	return raw
}

func TestAuthConfigValidate(t *testing.T) {
	tests := []struct {
		name       string
		cfg        config.AuthConfig
		wantErrMsg string
	}{
		{name: "valid config", cfg: config.AuthConfig{JWTSecret: "x", Issuer: "issuer", Audience: "audience"}, wantErrMsg: ""},
		{name: "missing jwt secret", cfg: config.AuthConfig{JWTSecret: "", Issuer: "issuer", Audience: "audience"}, wantErrMsg: "auth config JWTSecret is required"},
		{name: "missing issuer", cfg: config.AuthConfig{JWTSecret: "x", Issuer: "", Audience: "audience"}, wantErrMsg: "auth config Issuer is required"},
		{name: "missing audience", cfg: config.AuthConfig{JWTSecret: "x", Issuer: "issuer", Audience: ""}, wantErrMsg: "auth config Audience is required"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.wantErrMsg == "" {
				require.NoError(t, err)
				return
			}

			require.EqualError(t, err, tc.wantErrMsg)
		})
	}
}

func TestParseBearerToken(t *testing.T) {
	tests := []struct {
		name      string
		header    string
		wantToken string
		wantErr   error
	}{
		{name: "valid canonical header", header: "Bearer abc.def.ghi", wantToken: "abc.def.ghi", wantErr: nil},
		{name: "valid lowercase scheme", header: "bearer token123", wantToken: "token123", wantErr: nil},
		{name: "valid with extra spacing", header: "   Bearer    token123   ", wantToken: "token123", wantErr: nil},
		{name: "missing header", header: "", wantToken: "", wantErr: errMissingAuthorizationHeader},
		{name: "whitespace-only header", header: " \t  ", wantToken: "", wantErr: errMissingAuthorizationHeader},
		{name: "missing token", header: "Bearer", wantToken: "", wantErr: errMalformedAuthorizationHeader},
		{name: "wrong scheme", header: "Basic abc.def", wantToken: "", wantErr: errMalformedAuthorizationHeader},
		{name: "too many parts", header: "Bearer one two", wantToken: "", wantErr: errMalformedAuthorizationHeader},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			gotToken, err := parseBearerToken(tc.header)
			if tc.wantErr == nil {
				require.NoError(t, err)
				require.Equal(t, tc.wantToken, gotToken)
				return
			}

			require.Error(t, err)
			require.ErrorIs(t, err, tc.wantErr)
			require.Empty(t, gotToken)
		})
	}
}

func TestParseAndValidateClaims(t *testing.T) {
	now := time.Now()
	baseCfg := defaultAuthCfg()
	baseClaims := validClaims(now)

	wrongIssuerClaims := baseClaims
	wrongIssuerClaims.Issuer = "other-issuer"

	wrongAudienceClaims := baseClaims
	wrongAudienceClaims.Audience = jwt.ClaimStrings{"other-audience"}

	expiredClaims := baseClaims
	expiredClaims.ExpiresAt = jwt.NewNumericDate(now.Add(-1 * time.Minute))

	missingExpClaims := baseClaims
	missingExpClaims.ExpiresAt = nil

	missingSubClaims := baseClaims
	missingSubClaims.Subject = ""

	missingIdentityKeyInSubClaims := baseClaims
	missingIdentityKeyInSubClaims.Subject = "github:"

	invalidSubjectPrefixClaims := baseClaims
	invalidSubjectPrefixClaims.Subject = "user:" + testIdentityKey

	opaqueSubjectWithIdentityKeyClaim := baseClaims
	opaqueSubjectWithIdentityKeyClaim.Subject = "github:v2:opaquehash"
	opaqueSubjectWithIdentityKeyClaim.IdentityKeyClaim = testIdentityKey

	tests := []struct {
		name        string
		token       string
		cfg         config.AuthConfig
		wantErr     bool
		wantSubject string
	}{
		{name: "valid token", token: mintToken(t, testJWTSecret, baseClaims), cfg: baseCfg, wantErr: false, wantSubject: "github:" + testIdentityKey},
		{name: "opaque subject with identity_key claim", token: mintToken(t, testJWTSecret, opaqueSubjectWithIdentityKeyClaim), cfg: baseCfg, wantErr: false, wantSubject: opaqueSubjectWithIdentityKeyClaim.Subject},
		{name: "wrong secret", token: mintToken(t, "other-secret", baseClaims), cfg: baseCfg, wantErr: true},
		{name: "wrong issuer", token: mintToken(t, testJWTSecret, wrongIssuerClaims), cfg: baseCfg, wantErr: true},
		{name: "wrong audience", token: mintToken(t, testJWTSecret, wrongAudienceClaims), cfg: baseCfg, wantErr: true},
		{name: "expired token", token: mintToken(t, testJWTSecret, expiredClaims), cfg: baseCfg, wantErr: true},
		{name: "missing exp claim", token: mintToken(t, testJWTSecret, missingExpClaims), cfg: baseCfg, wantErr: true},
		{name: "missing sub claim", token: mintToken(t, testJWTSecret, missingSubClaims), cfg: baseCfg, wantErr: true},
		{name: "missing identity key in subject", token: mintToken(t, testJWTSecret, missingIdentityKeyInSubClaims), cfg: baseCfg, wantErr: true},
		{name: "invalid subject prefix", token: mintToken(t, testJWTSecret, invalidSubjectPrefixClaims), cfg: baseCfg, wantErr: true},
		{name: "wrong signing method", token: mintTokenWithMethod(t, jwt.SigningMethodHS384, testJWTSecret, baseClaims), cfg: baseCfg, wantErr: true},
		{name: "malformed token", token: "not-a-jwt", cfg: baseCfg, wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			claims, err := parseAndValidateClaims(tc.token, tc.cfg)
			if tc.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, errInvalidToken)
				require.Nil(t, claims)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, claims)
			require.Equal(t, tc.cfg.Issuer, claims.Issuer)
			require.Equal(t, testIdentityKey, claims.IdentityKey)
			require.Equal(t, tc.wantSubject, claims.Subject)
		})
	}
}

func TestKeyByIdentityKey(t *testing.T) {
	t.Run("falls back when principal missing", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "/", nil)
		require.NoError(t, err)

		key, err := KeyByIdentityKey(req)

		require.NoError(t, err)
		assert.Equal(t, "identity:unknown", key)
	})

	t.Run("falls back when identity key empty", func(t *testing.T) {
		req, err := http.NewRequestWithContext(withPrincipal(context.Background(), Principal{}), http.MethodGet, "/", nil)
		require.NoError(t, err)

		key, err := KeyByIdentityKey(req)

		require.NoError(t, err)
		assert.Equal(t, "identity:unknown", key)
	})

	t.Run("uses principal identity key", func(t *testing.T) {
		req, err := http.NewRequestWithContext(withPrincipal(context.Background(), Principal{IdentityKey: "user-123"}), http.MethodGet, "/", nil)
		require.NoError(t, err)

		key, err := KeyByIdentityKey(req)

		require.NoError(t, err)
		assert.Equal(t, "identity:user-123", key)
	})
}
