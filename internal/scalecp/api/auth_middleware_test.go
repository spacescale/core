// Copyright (c) 2026 SpaceScale Systems Inc. All rights reserved.

// This file contains white-box tests for auth helper behavior.
// It keeps token/config parsing validation close to middleware internals, while
// endpoint-level HTTP contract tests live in separate integration suites.

package api

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/spacescale/core/internal/shared/config"
	"github.com/stretchr/testify/require"
)

const (
	testJWTSecret   = "test-bff-secret"    // shared test signing secret.
	testJWTIssuer   = "spacescale-web-bff" // expected issuer in auth middleware tests.
	testJWTAudience = "spacescale-api"     // expected audience in auth middleware tests.
	testIdentityKey = "t0gun"              // canonical identity key used in test claims.
)

// testJWTClaims models claims expected by middleware tests.
// Subject is the identity source of truth and follows github:<identity-key>
// format,
// while embedded RegisteredClaims carry standard JWT fields.
type testJWTClaims struct {
	IdentityKeyClaim string `json:"identity_key,omitempty"`
	jwt.RegisteredClaims
}

// defaultAuthCfg returns a valid baseline verification config used in tests.
// Individual test cases can copy/override fields to verify issuer/audience/
// signature mismatch behavior without rebuilding full structs each time.
func defaultAuthCfg() config.AuthConfig {
	return config.AuthConfig{
		JWTSecret: testJWTSecret,
		Issuer:    testJWTIssuer,
		Audience:  testJWTAudience,
	}
}

// validClaims builds a known-good claim payload for happy-path token tests.
// It includes all fields required by parseAndValidateClaims:
// - sub
// - iss
// - aud
// - exp (required by WithExpirationRequired)
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

// mintToken signs test claims with HS256 and returns a raw JWT string.
// This helper keeps token creation one-line in table setup while ensuring all
// signing errors fail the current test immediately.
func mintToken(t *testing.T, secret string, claims testJWTClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	raw, err := token.SignedString([]byte(secret))
	require.NoError(t, err)
	return raw
}

// mintTokenWithMethod signs claims with an explicit algorithm.
// It is used for negative-path tests that verify method restrictions, e.g.
// tokens signed with HS384 should be rejected when middleware only allows HS256.
func mintTokenWithMethod(t *testing.T, method jwt.SigningMethod, secret string, claims testJWTClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(method, claims)
	raw, err := token.SignedString([]byte(secret))
	require.NoError(t, err)
	return raw
}

// TestAuthConfigValidate verifies required startup auth configuration checks.
// These are pure function cases and ensure misconfiguration is caught before
// middleware starts handling live requests.
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

// TestParseBearerToken verifies helper-level Authorization header parsing.
// The cases cover valid bearer inputs and malformed edge conditions so callers
// can rely on consistent token extraction and error classification.
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

// TestParseAndValidateClaims verifies JWT parsing and claim-validation rules.
// It covers the happy path plus negative cases (signature mismatch, issuer/
// audience mismatch, expired/missing required claims, wrong algorithm, and
// malformed token) so helper behavior stays stable under refactors.
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
