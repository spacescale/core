// This file provides shared integration test helpers for HTTP API tests.
// It hides setup boilerplate for database connectivity, service wiring, and
// httptest server lifecycle so individual test cases can stay focused.
// The request helper standardizes call execution and response body capture,
// which keeps assertions consistent across endpoint test files.
// Update this file when test infrastructure changes affect multiple test suites.

// Package http_api_test provides shared HTTP test helpers.
package http_api_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/t0gun/spacescale/internal/http_api"
	pgstore "github.com/t0gun/spacescale/internal/postgres/gen"
	"github.com/t0gun/spacescale/internal/service"
)

type testServer struct {
	server *httptest.Server
	pool   *pgxpool.Pool
}

// newTestServer creates an integration test server backed by a real database.
// Tests are skipped when TEST_DATABASE_URL is missing, otherwise storage,
// service, and HTTP layers are initialized with production-like wiring.
func newTestServer(t *testing.T) *testServer {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

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

	queries := pgstore.New(pool)
	svc := service.NewProjectService(queries)
	api := http_api.NewServer(svc)

	return &testServer{
		server: httptest.NewServer(api.Router()),
		pool:   pool,
	}
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
	req, err := http.NewRequest(method, ts.server.URL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
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
