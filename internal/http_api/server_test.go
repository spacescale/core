package http_api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/t0gun/paas/internal/adapters/runtime/fake"
	"github.com/t0gun/paas/internal/adapters/store"
	"github.com/t0gun/paas/internal/http_api"
	"github.com/t0gun/paas/internal/service"
)

func newTestServer(t *testing.T) (*httptest.Server, *store.MemoryStore) {
	t.Helper()
	st := store.NewMemoryStore()
	rt := fake.New("spacescale.ai")
	svc := service.NewAppServiceWithRuntime(st, rt)
	api := http_api.NewServer(svc)
	ts := httptest.NewServer(api.Router())
	return ts, st
}

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

func newJSONRequest(t *testing.T, method, url string, body []byte) *http.Request {
	t.Helper()
	req := newRequest(t, method, url, body)
	req.Header.Set("Content-Type", "application/json")
	return req
}

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

func TestHealthz(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()
	req := newRequest(t, http.MethodGet, ts.URL+"/healthz", nil)
	res := doRequest(t, req)
	assert.Equal(t, http.StatusOK, res.StatusCode)
}

func TestCreateApp(t *testing.T) {
	t.Run("valid - 201", func(t *testing.T) {
		ts, _ := newTestServer(t)
		defer ts.Close()

		body := map[string]any{"name": "hello", "image": "nginx:latest", "port": 8080}
		reqBody, err := json.Marshal(body)
		assert.NoError(t, err)

		req := newJSONRequest(t, http.MethodPost, ts.URL+"/v0/apps", reqBody)
		res := doRequest(t, req)
		assert.Equal(t, http.StatusCreated, res.StatusCode)

		var got map[string]any
		assert.NoError(t, json.NewDecoder(res.Body).Decode(&got))
		assert.NotEmpty(t, got["id"])
		assert.Equal(t, "hello", got["name"])
		assert.Equal(t, "nginx:latest", got["image"])
	})

	t.Run("invalid - 400", func(t *testing.T) {
		ts, _ := newTestServer(t)
		defer ts.Close()

		body := map[string]any{"name": "Bad_Name", "image": "nginx:latest", "port": 8080}
		reqBody, err := json.Marshal(body)
		assert.NoError(t, err)

		req := newJSONRequest(t, http.MethodPost, ts.URL+"/v0/apps", reqBody)
		res := doRequest(t, req)
		assert.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

	t.Run("invalid json - 400", func(t *testing.T) {
		ts, _ := newTestServer(t)
		defer ts.Close()

		reqBody := []byte(`{"name": "hello",`)
		req := newJSONRequest(t, http.MethodPost, ts.URL+"/v0/apps", reqBody)
		res := doRequest(t, req)
		assert.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

}

func TestCreateAppConflict(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()
	body := `{"name":"hello","image":"nginx:latest","port":8080}`

	// create first
	req1 := newJSONRequest(t, http.MethodPost, ts.URL+"/v0/apps", []byte(body))
	res1 := doRequest(t, req1)
	assert.Equal(t, http.StatusCreated, res1.StatusCode)

	// create again with same name => 409
	req2 := newJSONRequest(t, http.MethodPost, ts.URL+"/v0/apps", []byte(body))
	res2 := doRequest(t, req2)

	assert.Equal(t, http.StatusConflict, res2.StatusCode)
}

func TestDeployAndProcessAndListDeployments(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	// create app
	createReq := `{"name":"hello","image":"nginx:latest","port":8080}`
	req := newJSONRequest(t, http.MethodPost, ts.URL+"/v0/apps", []byte(createReq))
	res := doRequest(t, req)
	assert.Equal(t, http.StatusCreated, res.StatusCode)

	var created map[string]any
	assert.NoError(t, json.NewDecoder(res.Body).Decode(&created))

	appID, _ := created["id"].(string)
	assert.NotEmpty(t, appID)

	// deploy => 202
	deployReq := newRequest(t, http.MethodPost, ts.URL+"/v0/apps/"+appID+"/deploy", nil)
	deployRes := doRequest(t, deployReq)
	assert.Equal(t, http.StatusAccepted, deployRes.StatusCode)

	// process => 200
	processReq := newRequest(t, http.MethodPost, ts.URL+"/v0/deployments/next:process", nil)
	processRes := doRequest(t, processReq)
	assert.Equal(t, http.StatusOK, processRes.StatusCode)

	var dep map[string]any
	assert.NoError(t, json.NewDecoder(processRes.Body).Decode(&dep))

	assert.Equal(t, "RUNNING", dep["status"])
	assert.NotEmpty(t, dep["url"])

	// list => 200 and includes deployment
	listReq := newRequest(t, http.MethodGet, ts.URL+"/v0/apps/"+appID+"/deployments", nil)
	listRes := doRequest(t, listReq)

	assert.Equal(t, http.StatusOK, listRes.StatusCode)

	var deps []map[string]any
	assert.NoError(t, json.NewDecoder(listRes.Body).Decode(&deps))
	assert.Len(t, deps, 1)
	assert.Equal(t, "RUNNING", deps[0]["status"])
}

func TestDeployMissingApp(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	req := newRequest(t, http.MethodPost, ts.URL+"/v0/apps/missing/deploy", nil)
	res := doRequest(t, req)

	assert.Equal(t, http.StatusNotFound, res.StatusCode)
}

func TestProcessNoWork(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	req := newRequest(t, http.MethodPost, ts.URL+"/v0/deployments/next:process", nil)
	res := doRequest(t, req)

	assert.Equal(t, http.StatusNoContent, res.StatusCode)
}

func TestListApps(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()
	req := newRequest(t, http.MethodGet, ts.URL+"/v0/apps", nil)
	res := doRequest(t, req)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	var empty []map[string]any
	assert.NoError(t, json.NewDecoder(res.Body).Decode(&empty))
	assert.Len(t, empty, 0)

	// create one app
	createReq := `{"name":"hello","image":"nginx:latest","port":8080}`
	req0 := newJSONRequest(t, http.MethodPost, ts.URL+"/v0/apps", []byte(createReq))
	res0 := doRequest(t, req0)
	assert.Equal(t, http.StatusCreated, res0.StatusCode)
	_ = res.Body.Close()

	req1 := newRequest(t, http.MethodGet, ts.URL+"/v0/apps", nil)
	res1 := doRequest(t, req1)
	assert.Equal(t, http.StatusOK, res1.StatusCode)

	var apps []map[string]any
	assert.NoError(t, json.NewDecoder(res1.Body).Decode(&apps))
	assert.Len(t, apps, 1)
	assert.Equal(t, "hello", apps[0]["name"])
}
