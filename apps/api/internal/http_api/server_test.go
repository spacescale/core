// Tests for http api routes and responses
// Tests exercise app creation and deployment flows
// Tests verify status codes and response fields
// Tests cover exposure disabled behavior
// These tests guard handler regressions

package http_api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/t0gun/spacescale/internal/adapters/runtime/docker"
	"github.com/t0gun/spacescale/internal/adapters/store"
	"github.com/t0gun/spacescale/internal/http_api"
	"github.com/t0gun/spacescale/internal/service"
)

// newTestServer builds a test server and backing store.
func newTestServer(t *testing.T, workerToken string) (*httptest.Server, *store.MemoryStore) {
	t.Helper()

	st := store.NewMemoryStore()
	rt, _ := docker.New(docker.WithNamePrefix("spacescale-http-api-"))
	svc := service.NewAppServiceWithRuntime(st, rt)

	api := http_api.NewServer(svc, workerToken)
	ts := httptest.NewServer(api.Router())

	return ts, st
}

// newRequest builds an HTTP request with an optional body.
func newRequest(t *testing.T, method, url string, body []byte) *http.Request {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	return req
}

// newJSONRequest builds an HTTP JSON request.
func newJSONRequest(t *testing.T, method, url string, body []byte) *http.Request {
	t.Helper()
	req := newRequest(t, method, url, body)
	req.Header.Set("Content-Type", "application/json")
	return req
}

// doRequest executes a request and registers cleanup.
func doRequest(t *testing.T, req *http.Request) *http.Response {
	t.Helper()
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	t.Cleanup(func() {
		assert.NoError(t, res.Body.Close())
	})
	return res
}

// createApp creates an app via the API and returns the response body.
func createApp(t *testing.T, ts *httptest.Server, name, image string, port *int, expose *bool, env map[string]string) map[string]any {
	t.Helper()
	body := map[string]any{"name": name, "image": image}
	if port != nil {
		body["port"] = *port
	}
	if expose != nil {
		body["expose"] = *expose
	}
	if env != nil {
		body["env"] = env
	}
	reqBody, err := json.Marshal(body)
	assert.NoError(t, err)

	req := newJSONRequest(t, http.MethodPost, ts.URL+"/v0/apps", reqBody)
	res := doRequest(t, req)
	assert.Equal(t, http.StatusCreated, res.StatusCode)

	var created map[string]any
	assert.NoError(t, json.NewDecoder(res.Body).Decode(&created))
	return created
}

// TestHealthz verifies the health endpoint.
func TestHealthz(t *testing.T) {
	ts, _ := newTestServer(t, "")
	defer ts.Close()
	req := newRequest(t, http.MethodGet, ts.URL+"/healthz", nil)
	res := doRequest(t, req)
	assert.Equal(t, http.StatusOK, res.StatusCode)
}

// TestCreateApp validates app creation responses.
func TestCreateApp(t *testing.T) {
	t.Run("valid - 201", func(t *testing.T) {
		ts, _ := newTestServer(t, "")
		defer ts.Close()

		got := createApp(t, ts, "hello", "nginx:latest", ptrInt(8080), nil, nil)
		assert.NotEmpty(t, got["id"])
		assert.Equal(t, "hello", got["name"])
		assert.Equal(t, "nginx:latest", got["image"])
	})

	t.Run("invalid - 400", func(t *testing.T) {
		ts, _ := newTestServer(t, "")
		defer ts.Close()

		body := map[string]any{"name": "Bad_Name", "image": "nginx:latest", "port": 8080}
		reqBody, err := json.Marshal(body)
		assert.NoError(t, err)

		req := newJSONRequest(t, http.MethodPost, ts.URL+"/v0/apps", reqBody)
		res := doRequest(t, req)
		assert.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

	t.Run("invalid json - 400", func(t *testing.T) {
		ts, _ := newTestServer(t, "")
		defer ts.Close()

		reqBody := []byte(`{"name": "hello",`)
		req := newJSONRequest(t, http.MethodPost, ts.URL+"/v0/apps", reqBody)
		res := doRequest(t, req)
		assert.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

}

// TestCreateAppConflict verifies conflict on duplicate names.
func TestCreateAppConflict(t *testing.T) {
	ts, _ := newTestServer(t, "")
	defer ts.Close()

	// create first
	createApp(t, ts, "hello", "nginx:latest", ptrInt(8080), nil, nil)

	// create again with same name
	body := `{"name":"hello","image":"nginx:latest","port":8080}`
	req2 := newJSONRequest(t, http.MethodPost, ts.URL+"/v0/apps", []byte(body))
	res2 := doRequest(t, req2)

	assert.Equal(t, http.StatusConflict, res2.StatusCode)
}

// TestDeployAndProcessAndListDeployments verifies the deploy processing flow.
func TestDeployAndProcessAndListDeployments(t *testing.T) {
	ts, _ := newTestServer(t, "")
	defer ts.Close()

	// create app
	created := createApp(t, ts, "hello", "nginx:latest", ptrInt(8080), nil, nil)
	appID, _ := created["id"].(string)
	assert.NotEmpty(t, appID)

	// deploy
	deployReq := newRequest(t, http.MethodPost, ts.URL+"/v0/apps/"+appID+"/deploy", nil)
	deployRes := doRequest(t, deployReq)
	assert.Equal(t, http.StatusAccepted, deployRes.StatusCode)

	// process
	processReq := newRequest(t, http.MethodPost, ts.URL+"/v0/deployments/next:process", nil)
	processRes := doRequest(t, processReq)
	assert.Equal(t, http.StatusOK, processRes.StatusCode)

	var dep map[string]any
	assert.NoError(t, json.NewDecoder(processRes.Body).Decode(&dep))

	assert.Equal(t, "RUNNING", dep["status"])
	assert.NotEmpty(t, dep["url"])

	// list and includes deployment
	listReq := newRequest(t, http.MethodGet, ts.URL+"/v0/apps/"+appID+"/deployments", nil)
	listRes := doRequest(t, listReq)

	assert.Equal(t, http.StatusOK, listRes.StatusCode)

	var deps []map[string]any
	assert.NoError(t, json.NewDecoder(listRes.Body).Decode(&deps))
	assert.Len(t, deps, 1)
	assert.Equal(t, "RUNNING", deps[0]["status"])
}

// TestDeployNoExpose verifies deployments without exposure omit URLs.
func TestDeployNoExpose(t *testing.T) {
	ts, _ := newTestServer(t, "")
	defer ts.Close()

	expose := false
	created := createApp(t, ts, "hello", "nginx:latest", nil, &expose, nil)
	appID, _ := created["id"].(string)
	assert.NotEmpty(t, appID)

	deployReq := newRequest(t, http.MethodPost, ts.URL+"/v0/apps/"+appID+"/deploy", nil)
	deployRes := doRequest(t, deployReq)
	assert.Equal(t, http.StatusAccepted, deployRes.StatusCode)

	processReq := newRequest(t, http.MethodPost, ts.URL+"/v0/deployments/next:process", nil)
	processRes := doRequest(t, processReq)
	assert.Equal(t, http.StatusOK, processRes.StatusCode)

	var dep map[string]any
	assert.NoError(t, json.NewDecoder(processRes.Body).Decode(&dep))
	assert.Equal(t, "RUNNING", dep["status"])
	_, ok := dep["url"]
	assert.False(t, ok)
}

// TestDeployMissingApp verifies missing apps return 404.
func TestDeployMissingApp(t *testing.T) {
	ts, _ := newTestServer(t, "")
	defer ts.Close()

	req := newRequest(t, http.MethodPost, ts.URL+"/v0/apps/missing/deploy", nil)
	res := doRequest(t, req)

	assert.Equal(t, http.StatusNotFound, res.StatusCode)
}

// TestListDeploymentsMissingApp verifies missing apps return 404.
func TestListDeploymentsMissingApp(t *testing.T) {
	ts, _ := newTestServer(t, "")
	defer ts.Close()

	req := newRequest(t, http.MethodGet, ts.URL+"/v0/apps/missing/deployments", nil)
	res := doRequest(t, req)

	assert.Equal(t, http.StatusNotFound, res.StatusCode)
}

// TestProcessNoWork verifies no queued work returns 204.
func TestProcessNoWork(t *testing.T) {
	ts, _ := newTestServer(t, "")
	defer ts.Close()

	req := newRequest(t, http.MethodPost, ts.URL+"/v0/deployments/next:process", nil)
	res := doRequest(t, req)

	assert.Equal(t, http.StatusNoContent, res.StatusCode)
}

// TestProcessNoRuntime verifies missing runtime returns 503.
func TestProcessNoRuntime(t *testing.T) {
	st := store.NewMemoryStore()
	svc := service.NewAppService(st)
	api := http_api.NewServer(svc, "")
	ts := httptest.NewServer(api.Router())
	defer ts.Close()

	req := newRequest(t, http.MethodPost, ts.URL+"/v0/deployments/next:process", nil)
	res := doRequest(t, req)

	assert.Equal(t, http.StatusServiceUnavailable, res.StatusCode)

	var got map[string]any
	assert.NoError(t, json.NewDecoder(res.Body).Decode(&got))
	assert.Equal(t, "runtime not configured", got["error"])
}

// TestListApps verifies app listing behavior.
func TestListApps(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		ts, _ := newTestServer(t, "")
		defer ts.Close()

		req := newRequest(t, http.MethodGet, ts.URL+"/v0/apps", nil)
		res := doRequest(t, req)

		assert.Equal(t, http.StatusOK, res.StatusCode)

		var got []map[string]any
		assert.NoError(t, json.NewDecoder(res.Body).Decode(&got))
		assert.Len(t, got, 0)
	})

	t.Run("list includes created app", func(t *testing.T) {
		ts, _ := newTestServer(t, "")
		defer ts.Close()

		created := createApp(t, ts, "hello", "nginx:latest", ptrInt(8080), nil, nil)

		req := newRequest(t, http.MethodGet, ts.URL+"/v0/apps", nil)
		res := doRequest(t, req)

		assert.Equal(t, http.StatusOK, res.StatusCode)

		var got []map[string]any
		assert.NoError(t, json.NewDecoder(res.Body).Decode(&got))
		assert.GreaterOrEqual(t, len(got), 1)
		assert.Equal(t, created["id"], got[0]["id"])
	})
}

// TestGetAppByID verifies app lookup behavior.
func TestGetAppByID(t *testing.T) {
	t.Run("ok - 200", func(t *testing.T) {
		ts, _ := newTestServer(t, "")
		defer ts.Close()

		created := createApp(t, ts, "hello", "nginx:latest", ptrInt(8080), nil, nil)
		appID, _ := created["id"].(string)
		assert.NotEmpty(t, appID)

		getReq := newRequest(t, http.MethodGet, ts.URL+"/v0/apps/"+appID, nil)
		getRes := doRequest(t, getReq)
		assert.Equal(t, http.StatusOK, getRes.StatusCode)

		var got map[string]any
		assert.NoError(t, json.NewDecoder(getRes.Body).Decode(&got))
		assert.Equal(t, appID, got["id"])
		assert.Equal(t, "hello", got["name"])
		assert.Equal(t, "nginx:latest", got["image"])
	})

	t.Run("not found - 404", func(t *testing.T) {
		ts, _ := newTestServer(t, "")
		defer ts.Close()

		req := newRequest(t, http.MethodGet, ts.URL+"/v0/apps/missing", nil)
		res := doRequest(t, req)
		assert.Equal(t, http.StatusNotFound, res.StatusCode)

		var got map[string]any
		assert.NoError(t, json.NewDecoder(res.Body).Decode(&got))
		assert.Equal(t, "not found", got["error"])
	})
}

// TestWorkerAuth_ProcessNextDeployment verifies worker token enforcement.
func TestWorkerAuth_ProcessNextDeployment(t *testing.T) {
	tests := []struct {
		label       string
		serverToken string
		headerToken string
		wantCode    int
	}{
		{label: "token disabled", serverToken: "", headerToken: "", wantCode: http.StatusNoContent},
		{label: "token enabled - missing header", serverToken: "secret", headerToken: "", wantCode: http.StatusUnauthorized},
		{label: "token enabled wrong header", serverToken: "secret", headerToken: "nope", wantCode: http.StatusUnauthorized},
		{label: "token enabled correct header", serverToken: "secret", headerToken: "secret", wantCode: http.StatusNoContent},
	}

	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			ts, _ := newTestServer(t, tt.serverToken)
			defer ts.Close()

			req := newRequest(t, http.MethodPost, ts.URL+"/v0/deployments/next:process", nil)
			if tt.headerToken != "" {
				req.Header.Set("X-Worker-Token", tt.headerToken)
			}

			res := doRequest(t, req)
			assert.Equal(t, tt.wantCode, res.StatusCode)
		})
	}
}

// ptrInt returns a pointer to v.
func ptrInt(v int) *int {
	return &v
}
