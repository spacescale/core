// This file documents white-box test intent for auth middleware helper logic.
// It exists as a test-layer placeholder so auth-focused unit cases can be added
// incrementally without mixing them into integration-heavy endpoint suites.
//
// Planned white-box suites in this file:
// - AuthConfig.Validate: required runtime configuration checks.
// - parseBearerToken: Authorization header parsing and malformed edge cases.
// - parseAndValidateClaims: JWT claim validation regressions (exp/sub/github_id,
//   issuer, audience, signing method expectations).
//
// Why this separation is useful:
// - Keeps helper-level auth behavior tests fast and deterministic.
// - Makes utility regressions easier to detect during middleware refactors.
// - Preserves clear boundaries between helper-unit tests and HTTP contract tests.

package http_api

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

// Test constants mirror the middleware defaults/config used in local development.
// Keeping them centralized makes each test case easier to read and avoids repeated
// string literals across helper and table-driven setups.
const (
	testJWTSecret   = "test-bff-secret"
	testJWTIssuer   = "spacescale-web-bff"
	testJWTAudience = "spacescale-api"
	testGithubID    = "t0gun"
)

// testJWTClaims models the custom and registered claims expected by middleware.
// The custom github_id field is required by parseAndValidateClaims, while the
// embedded RegisteredClaims carry standard JWT fields (sub/iss/aud/exp/iat).
type testJWTClaims struct {
	GithubID string `json:"github_id"`
	jwt.RegisteredClaims
}

// defaultAuthCfg returns a valid baseline verification config used in tests.
// Individual test cases can copy/override fields to verify issuer/audience/
// signature mismatch behavior without rebuilding full structs each time.
func defaultAuthCfg() AuthConfig {
	return AuthConfig{
		JWTSecret: testJWTSecret,
		Issuer:    testJWTIssuer,
		Audience:  testJWTAudience,
	}
}

// validClaims builds a known-good claim payload for happy-path token tests.
// It includes all fields required by parseAndValidateClaims:
// - sub
// - github_id
// - iss
// - aud
// - exp (required by WithExpirationRequired)
func validClaims(now time.Time) testJWTClaims {
	return testJWTClaims{
		GithubID: testGithubID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "github:" + testGithubID,
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
		cfg        AuthConfig
		wantErrMsg string
	}{
		{name: "valid config", cfg: AuthConfig{JWTSecret: "x", Issuer: "issuer", Audience: "audience"}, wantErrMsg: ""},
		{name: "missing jwt secret", cfg: AuthConfig{JWTSecret: "", Issuer: "issuer", Audience: "audience"}, wantErrMsg: "auth config JWTSecret is required"},
		{name: "missing issuer", cfg: AuthConfig{JWTSecret: "x", Issuer: "", Audience: "audience"}, wantErrMsg: "auth config Issuer is required"},
		{name: "missing audience", cfg: AuthConfig{JWTSecret: "x", Issuer: "issuer", Audience: ""}, wantErrMsg: "auth config Audience is required"},
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

	missingGithubClaims := baseClaims
	missingGithubClaims.GithubID = ""

	tests := []struct {
		name    string
		token   string
		cfg     AuthConfig
		wantErr bool
	}{
		{name: "valid token", token: mintToken(t, testJWTSecret, baseClaims), cfg: baseCfg, wantErr: false},
		{name: "wrong secret", token: mintToken(t, "other-secret", baseClaims), cfg: baseCfg, wantErr: true},
		{name: "wrong issuer", token: mintToken(t, testJWTSecret, wrongIssuerClaims), cfg: baseCfg, wantErr: true},
		{name: "wrong audience", token: mintToken(t, testJWTSecret, wrongAudienceClaims), cfg: baseCfg, wantErr: true},
		{name: "expired token", token: mintToken(t, testJWTSecret, expiredClaims), cfg: baseCfg, wantErr: true},
		{name: "missing exp claim", token: mintToken(t, testJWTSecret, missingExpClaims), cfg: baseCfg, wantErr: true},
		{name: "missing sub claim", token: mintToken(t, testJWTSecret, missingSubClaims), cfg: baseCfg, wantErr: true},
		{name: "missing github_id claim", token: mintToken(t, testJWTSecret, missingGithubClaims), cfg: baseCfg, wantErr: true},
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
			require.Equal(t, testGithubID, claims.GithubID)
			require.Equal(t, "github:"+testGithubID, claims.Subject)
		})
	}
}
