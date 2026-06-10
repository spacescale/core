package api_test

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
	"github.com/spacescale/core/control/api"
	"github.com/spacescale/core/control/db/sqlc"
	"github.com/spacescale/core/control/service"
	"github.com/spacescale/core/control/service/tenant"
	"github.com/spacescale/core/shared/config"
	"github.com/stretchr/testify/require"
)

const (
	testJWTSecret          = "test-bff-secret"
	testIssuer             = "spacescale-web-bff-test"
	testAudience           = "spacescale-api-test"
	testEnvEncryptionKeyID = "test-key-v1"
	testEnvEncryptionKey   = "0123456789abcdef0123456789abcdef"
)

type testServer struct {
	server *httptest.Server
	pool   *pgxpool.Pool
}

type testJWTClaims struct {
	jwt.RegisteredClaims
}

type testResponse struct {
	StatusCode int
	Header     http.Header
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	require.NoError(t, err)
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		require.NoError(t, err)
	}

	authCfg := config.AuthConfig{Mode: config.AuthModeEnabled, JWTSecret: testJWTSecret, Issuer: testIssuer, Audience: testAudience}

	queries := sqlc.New(pool)
	svcs, err := service.NewServices(service.Deps{
		Queries:            queries,
		DBPool:             pool,
		EnvEncryptionKeyID: testEnvEncryptionKeyID,
		EnvEncryptionKey:   []byte(testEnvEncryptionKey),
	})
	if err != nil {
		pool.Close()
		require.NoError(t, err)
	}
	server := api.NewServer(api.ServerDeps{
		Services: svcs,
		DBPool:   pool,
		Config: config.Control{
			Auth: authCfg,
		},
	})

	return &testServer{server: httptest.NewServer(server.Router()), pool: pool}
}

func TestUnauthorizedRequestsStayUnauthorized(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	for range 3 {
		resp, data := doRequest(t, ts, http.MethodPost, "/v1/workspaces/00000000-0000-0000-0000-000000000000/projects", []byte(`{}`), map[string]string{
			"Content-Type": "application/json",
		})

		require.Equal(t, http.StatusUnauthorized, resp.StatusCode, string(data))

		var out errorResponse
		require.NoError(t, json.Unmarshal(data, &out))
		require.Equal(t, "unauthorized", out.Error)
	}
}

func syncAuthUserForTest(t *testing.T, ts *testServer, identityKey string) {
	t.Helper()
	queries := sqlc.New(ts.pool)
	users := tenant.NewUserService(queries)

	_, err := users.SyncAuthUser(context.Background(), tenant.SyncAuthUserParams{
		IdentityKey: identityKey,
		Email:       "dev@example.com",
		Name:        "Dev",
		AvatarURL:   "https://example.com/avatar.png",
	})
	require.NoError(t, err)
}

func createWorkspaceForIdentity(t *testing.T, ts *testServer, identityKey, name string) string {
	t.Helper()
	queries := sqlc.New(ts.pool)
	user, err := queries.GetUserByIdentityKey(context.Background(), identityKey)
	require.NoError(t, err)

	workspace, err := queries.CreateWorkspace(context.Background(), sqlc.CreateWorkspaceParams{
		OwnerUserID: user.ID,
		Name:        name,
	})
	require.NoError(t, err)
	return workspace.ID.String()
}

func (ts *testServer) close() {
	ts.server.Close()
	ts.pool.Close()
}

func doRequest(t *testing.T, ts *testServer, method, path string, body []byte, headers map[string]string) (testResponse, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), method, ts.server.URL+path, bytes.NewReader(body))
	require.NoError(t, err)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	return testResponse{StatusCode: resp.StatusCode, Header: resp.Header}, data
}

func authHeaderForIdentityKey(t *testing.T, identityKey string) string {
	t.Helper()
	claims := testJWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    testIssuer,
			Subject:   "github:" + identityKey,
			Audience:  jwt.ClaimStrings{testAudience},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(10 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-1 * time.Minute)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	raw, err := token.SignedString([]byte(testJWTSecret))
	require.NoError(t, err)
	return "Bearer " + raw
}

func uniqueIdentityKey(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("it-%d", time.Now().UnixNano())
}
