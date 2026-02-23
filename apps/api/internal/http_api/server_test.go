// This file contains server wiring tests and shared HTTP integration helpers.
// It covers configuration behavior and keeps test server setup reusable across
// endpoint suites without duplicating database/router bootstrapping code.

// Package http_api_test provides shared HTTP test helpers.
package http_api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/t0gun/spacescale/internal/config"
	"github.com/t0gun/spacescale/internal/http_api"
	pgstore "github.com/t0gun/spacescale/internal/postgres/gen"
	"github.com/t0gun/spacescale/internal/service"
)

const (
	testJWTSecret          = "test-bff-secret"         // shared test JWT secret for server auth middleware and token minting.
	testIssuer             = "spacescale-web-bff-test" // shared test token issuer.
	testAudience           = "spacescale-api-test"     // shared test token audience.
	testInternalAuthSecret = "test-internal-secret"
)

// testServer bundles resources needed by integration tests.
// server handles HTTP requests in-memory and pool connects to real test DB.
type testServer struct {
	server *httptest.Server
	pool   *pgxpool.Pool
}

// testJWTClaims matches claims expected by auth middleware.
// Subject carries canonical identity in github:<identity-key> format, while
// RegisteredClaims carries standard iss/aud/exp metadata.
type testJWTClaims struct {
	jwt.RegisteredClaims // embedded so claim fields are promoted to this type.
}

// TestDefaultRateLimitConfig verifies package-default limiter settings.
// This guards against accidental default drift in server wiring.
func TestDefaultRateLimitConfig(t *testing.T) {
	cfg := config.DefaultRateLimitConfig()

	require.Equal(t, 100, cfg.Requests)
	require.Equal(t, time.Minute, cfg.Window)
}

func TestDefaultInternalRateLimitConfig(t *testing.T) {
	globalCfg := config.DefaultInternalGlobalRateLimitConfig()
	identityCfg := config.DefaultInternalIdentityRateLimitConfig()

	require.Equal(t, 12000, globalCfg.Requests)
	require.Equal(t, time.Minute, globalCfg.Window)
	require.Equal(t, 30, identityCfg.Requests)
	require.Equal(t, time.Minute, identityCfg.Window)
}

// TestNewServerNormalizesInvalidRateLimitConfig verifies that invalid limiter
// values are normalized by server wiring so requests are not incorrectly
// blocked by zero-value configuration.
func TestNewServerNormalizesInvalidRateLimitConfig(t *testing.T) {
	ts := newTestServerWithRateLimitConfig(t, config.RateLimitConfig{})
	defer ts.close()
	syncAuthUserForTest(t, ts, "12345")

	name := fmt.Sprintf("normalize-rate-limit-%d", time.Now().UnixNano())
	workspaceName := fmt.Sprintf("workspace-%d", time.Now().UnixNano())
	workspaceID := createWorkspaceForIdentity(t, ts, "12345", workspaceName)
	body := []byte(fmt.Sprintf(`{"name":"%s","region":"global"}`, name))

	resp, data := doRequest(t, ts, http.MethodPost, "/v0/workspaces/"+workspaceID+"/projects", body, map[string]string{
		"Authorization": authHeaderForIdentityKey(t, "12345"),
		"Content-Type":  "application/json",
	})

	require.Equal(t, http.StatusCreated, resp.StatusCode, string(data))
	require.NotEqual(t, http.StatusTooManyRequests, resp.StatusCode, string(data))
}

// TestNewServerRequiresNonEmptyInternalSecret verifies startup validation for
// trusted internal endpoint protection.
func TestNewServerRequiresNonEmptyInternalSecret(t *testing.T) {
	require.PanicsWithValue(t, "http_api.NewServer requires non-empty internal auth secret", func() {
		http_api.NewServer(http_api.ServerDeps{
			Services: &service.Services{
				Projects: &service.ProjectService{},
				Users:    &service.UserService{},
			},
			DBPool: &pgxpool.Pool{},
			Config: config.APIConfig{
				InternalAuthSecret: "   ",
			},
		})
	})
}

// TestInternalAuthSyncHeaderValidation verifies trusted internal endpoint
// header checks for missing/invalid headers and the successful pass-through
// path when the header matches.
func TestInternalAuthSyncHeaderValidation(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	tests := []struct {
		name       string
		header     string
		wantStatus int
		wantErr    string
	}{
		{name: "missing header", header: "", wantStatus: http.StatusUnauthorized, wantErr: "unauthorized"},
		{name: "wrong header", header: "wrong-secret", wantStatus: http.StatusUnauthorized, wantErr: "unauthorized"},
		{name: "matching header after trim", header: "  " + testInternalAuthSecret + "  ", wantStatus: http.StatusBadRequest, wantErr: "invalid json"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			headers := map[string]string{"Content-Type": "application/json"}
			if tc.header != "" {
				headers["X-Internal-Auth"] = tc.header
			}

			resp, data := doRequest(t, ts, http.MethodPost, "/v0/internal/auth-sync", []byte("{"), headers)
			require.Equal(t, tc.wantStatus, resp.StatusCode, string(data))

			var out errorResponse
			require.NoError(t, json.Unmarshal(data, &out))
			require.Equal(t, tc.wantErr, out.Error)
		})
	}
}

// TestUnauthorizedRequestsAreNotRateLimited verifies middleware order for /v0
// routes: auth runs before user limiter, so unauthenticated requests are always
// rejected as unauthorized rather than sharing a limiter fallback bucket.
func TestUnauthorizedRequestsAreNotRateLimited(t *testing.T) {
	ts := newTestServerWithRateLimitConfig(t, config.RateLimitConfig{Requests: 1, Window: time.Minute})
	defer ts.close()

	for i := 0; i < 3; i++ {
		resp, data := doRequest(t, ts, http.MethodPost, "/v0/workspaces/00000000-0000-0000-0000-000000000000/projects", []byte(`{}`), map[string]string{
			"Content-Type": "application/json",
		})

		require.Equal(t, http.StatusUnauthorized, resp.StatusCode, string(data))

		var out errorResponse
		require.NoError(t, json.Unmarshal(data, &out))
		require.Equal(t, "unauthorized", out.Error)
	}
}

// TestInternalRequestsAreGloballyRateLimited verifies internal route circuit
// breaker behavior across different identity keys.
func TestInternalRequestsAreGloballyRateLimited(t *testing.T) {
	ts := newTestServerWithInternalRateLimitConfigs(
		t,
		config.RateLimitConfig{Requests: 1, Window: time.Minute},
		config.RateLimitConfig{Requests: 100, Window: time.Minute},
	)
	defer ts.close()

	firstIdentity := uniqueIdentityKey(t)
	syncAuthUserForTest(t, ts, firstIdentity)

	secondBody := []byte(fmt.Sprintf(`{"identityKey":"%s"}`, uniqueIdentityKey(t)))
	resp, data := doRequest(t, ts, http.MethodPost, "/v0/internal/auth-sync", secondBody, map[string]string{
		"X-Internal-Auth": testInternalAuthSecret,
		"Content-Type":    "application/json",
	})

	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode, string(data))

	var out errorResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.Equal(t, "rate limit exceeded", out.Error)
}

// newTestServer creates one integration server for the calling test.
// It uses package-default rate limiting.
// If TEST_DATABASE_URL is missing, the test is skipped.
func newTestServer(t *testing.T) *testServer {
	t.Helper()
	return newTestServerWithRateLimitConfigs(
		t,
		config.DefaultRateLimitConfig(),
		config.DefaultInternalGlobalRateLimitConfig(),
		config.DefaultInternalIdentityRateLimitConfig(),
	)
}

// newTestServerWithRateLimitConfig creates one integration server using the
// supplied rate-limit configuration.
// If TEST_DATABASE_URL is missing, the test is skipped.
func newTestServerWithRateLimitConfig(t *testing.T, rateLimitCfg config.RateLimitConfig) *testServer {
	t.Helper()
	return newTestServerWithRateLimitConfigs(
		t,
		rateLimitCfg,
		config.DefaultInternalGlobalRateLimitConfig(),
		config.DefaultInternalIdentityRateLimitConfig(),
	)
}

// newTestServerWithInternalRateLimitConfig creates one integration server with
// internal per-identity limiter override and default /v0 per-user limiting.
func newTestServerWithInternalRateLimitConfig(
	t *testing.T,
	internalIdentityCfg config.RateLimitConfig,
) *testServer {
	t.Helper()
	return newTestServerWithRateLimitConfigs(
		t,
		config.DefaultRateLimitConfig(),
		config.DefaultInternalGlobalRateLimitConfig(),
		internalIdentityCfg,
	)
}

// newTestServerWithInternalRateLimitConfigs creates one integration server with
// internal global and per-identity limiter overrides and default /v0 per-user
// limiting.
func newTestServerWithInternalRateLimitConfigs(
	t *testing.T,
	internalGlobalCfg config.RateLimitConfig,
	internalIdentityCfg config.RateLimitConfig,
) *testServer {
	t.Helper()
	return newTestServerWithRateLimitConfigs(
		t,
		config.DefaultRateLimitConfig(),
		internalGlobalCfg,
		internalIdentityCfg,
	)
}

// newTestServerWithRateLimitConfigs creates one integration server using all
// supplied limiter configuration values.
func newTestServerWithRateLimitConfigs(
	t *testing.T,
	rateLimitCfg config.RateLimitConfig,
	internalGlobalCfg config.RateLimitConfig,
	internalIdentityCfg config.RateLimitConfig,
) *testServer {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	// Use timeout-bounded setup so tests fail fast when DB is unavailable.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping db: %v", err)
	}

	// Configure auth middleware for this integration server instance.
	authCfg := config.AuthConfig{
		JWTSecret: testJWTSecret,
		Issuer:    testIssuer,
		Audience:  testAudience,
	}
	if err := authCfg.Validate(); err != nil {
		pool.Close()
		t.Fatalf("auth config: %v", err)
	}

	queries := pgstore.New(pool)
	svcs := service.NewServices(queries)
	api := http_api.NewServer(http_api.ServerDeps{
		Services: svcs,
		DBPool:   pool,
		Config: config.APIConfig{
			Auth:                      authCfg,
			RateLimit:                 rateLimitCfg,
			InternalGlobalRateLimit:   internalGlobalCfg,
			InternalIdentityRateLimit: internalIdentityCfg,
			LogPrivacy:                config.DefaultLogPrivacyConfig(),
			InternalAuthSecret:        testInternalAuthSecret,
		},
	})

	// httptest server exposes in-memory HTTP endpoint for black-box requests.
	return &testServer{
		server: httptest.NewServer(api.Router()),
		pool:   pool,
	}
}

// syncAuthUserForTest creates or updates a user through the trusted internal
// auth-sync endpoint so project tests can exercise the same lifecycle as real
// sign-in flows.
func syncAuthUserForTest(t *testing.T, ts *testServer, identityKey string) {
	t.Helper()

	body := []byte(fmt.Sprintf(
		`{"identityKey":"%s","email":"dev@example.com","name":"Dev","avatarUrl":"https://example.com/avatar.png"}`,
		identityKey,
	))

	resp, data := doRequest(t, ts, http.MethodPost, "/v0/internal/auth-sync", body, map[string]string{
		"X-Internal-Auth": testInternalAuthSecret,
		"Content-Type":    "application/json",
	})

	require.Equal(t, http.StatusOK, resp.StatusCode, string(data))
}

// createWorkspaceForIdentity inserts one workspace for an already synced user.
// It is used by project endpoint tests that require a workspace path parameter.
func createWorkspaceForIdentity(t *testing.T, ts *testServer, identityKey, name string) string {
	t.Helper()

	queries := pgstore.New(ts.pool)
	user, err := queries.GetUserByIdentityKey(context.Background(), identityKey)
	require.NoError(t, err)

	workspace, err := queries.CreateWorkspace(context.Background(), pgstore.CreateWorkspaceParams{
		OwnerUserID: user.ID,
		Name:        name,
	})
	require.NoError(t, err)

	return workspace.ID.String()
}

// close releases network and database resources used by the test server.
// It should be deferred in every test that allocates a server instance.
func (ts *testServer) close() {
	ts.server.Close()
	ts.pool.Close()
}

// doRequest sends one HTTP request to the in memory test server.
// Headers are applied as provided, and the raw response plus body bytes are
// returned so assertions can inspect both status and payload.
func doRequest(t *testing.T, ts *testServer, method, path string, body []byte, headers map[string]string) (*http.Response, []byte) {
	t.Helper()

	// Build request against test server URL.
	req, err := http.NewRequest(method, ts.server.URL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	// Apply caller-provided headers (auth/content-type/custom headers, etc).
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// Execute request and capture full body for assertions.
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp, data
}

// authHeaderForIdentityKey returns a valid bearer token header for integration tests.
//
// The token is signed with the same test secret configured in newTestServer,
// so auth middleware accepts it as a trusted principal.
//
// This helper keeps token construction in one place so endpoint tests can focus
// on request/response assertions instead of JWT plumbing.
func authHeaderForIdentityKey(t *testing.T, identityKey string) string {
	t.Helper()

	// Include required registered claims.
	claims := testJWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: testIssuer, // who created and signed the token. trusted token authority
			// Keep legacy "github:" prefix for current API subject compatibility.
			Subject:   "github:" + identityKey,
			Audience:  jwt.ClaimStrings{testAudience},                       // who is this token meant for
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(10 * time.Minute)), // tokens expire in 10 minutes
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-1 * time.Minute)), // token issued 1 minute ago
		},
	}

	// Sign with HS256 and return in "Authorization: Bearer <token>" format.
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	raw, err := token.SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatalf("sign auth token: %v", err)
	}
	return "Bearer " + raw
}

// uniqueIdentityKey returns a per-test identity key to avoid cross-test data
// collisions when integration tests share the same database instance.
func uniqueIdentityKey(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("it-%d", time.Now().UnixNano())
}
