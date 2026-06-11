// This file verifies end-to-end behavior of app HTTP workflows.
//
// Scope:
// - Request/response contracts for app create and list endpoints.
// - Initial status behavior (queued).
// - Persistence side effects in deployments, microvms, and app_env_vars tables.
//
// These are DB-backed integration tests by design so transport + service + SQL
// behavior are exercised together as one externally observable contract.

package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type createAppResponse struct {
	ID             string `json:"id"`
	ProjectID      string `json:"projectId"`
	Name           string `json:"name"`
	Slug           string `json:"slug"`
	Subdomain      string `json:"subdomain"`
	ImageRef       string `json:"imageRef"`
	TargetReplicas int32  `json:"targetReplicas"`
	PrimaryRegion  string `json:"primaryRegion"`
	RuntimePort    int32  `json:"runtimePort"`
	Status         string `json:"status"`
	IsPublic       bool   `json:"isPublic"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
}

type listAppsResponse struct {
	Apps []createAppResponse `json:"apps"`
}

// TestCreateAppCreatesQueuedDeployment verifies create-app writes app,
// deployment, and microvm state, returns queued status, and stores env vars.
func TestCreateAppCreatesQueuedDeployment(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	identityKey := uniqueIdentityKey(t)
	syncAuthUserForTest(t, ts, identityKey)

	workspaceID := createWorkspaceForIdentity(t, ts, identityKey, fmt.Sprintf("workspace-%d", time.Now().UnixNano()))
	project := createProjectViaAPI(t, ts, identityKey, workspaceID, fmt.Sprintf("project-%d", time.Now().UnixNano()))

	body := []byte(`{"name":"api","imageRef":"ghcr.io/acme/spacescale-api:latest","compute":{"vcpu":4,"memoryMb":4096,"dedicated":false},"primaryRegion":"ca-east","runtimePort":9090,"isPublic":true,"envVars":[{"key":"database_url","value":"postgres://local","isSecret":true}]}`)
	resp, data := doRequest(
		t,
		ts,
		http.MethodPost,
		fmt.Sprintf("/v1/workspaces/%s/projects/%s/apps", workspaceID, project.ID),
		body,
		map[string]string{
			"Cookie":       authCookieForIdentityKey(t, identityKey),
			"Content-Type": "application/json",
		},
	)

	require.Equal(t, http.StatusCreated, resp.StatusCode, string(data))

	var out createAppResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.NotEmpty(t, out.ID)
	require.Equal(t, project.ID, out.ProjectID)
	require.Equal(t, "api", out.Name)
	require.EqualValues(t, 1, out.TargetReplicas)
	require.Equal(t, "ca-east", out.PrimaryRegion)
	require.Equal(t, "queued", out.Status)
	require.EqualValues(t, 9090, out.RuntimePort)
	require.NotEmpty(t, resp.Header.Get("Location"))

	appID, err := uuid.Parse(out.ID)
	require.NoError(t, err)

	var appStatus string
	var appTargetReplicas int32
	err = ts.pool.QueryRow(context.Background(), `SELECT status, target_replicas FROM apps WHERE id = $1`, appID).Scan(&appStatus, &appTargetReplicas)
	require.NoError(t, err)
	require.Equal(t, "queued", appStatus)
	require.EqualValues(t, 1, appTargetReplicas)

	var deploymentID uuid.UUID
	var deploymentStatus string
	var deploymentImageRef string
	var deploymentRuntimePort int32
	var deploymentPublicURL *string
	err = ts.pool.QueryRow(
		context.Background(),
		`SELECT id, status, image_ref, runtime_port, public_url FROM deployments WHERE app_id = $1 ORDER BY created_at DESC LIMIT 1`,
		appID,
	).Scan(&deploymentID, &deploymentStatus, &deploymentImageRef, &deploymentRuntimePort, &deploymentPublicURL)
	require.NoError(t, err)
	require.Equal(t, "queued", deploymentStatus)
	require.Equal(t, "ghcr.io/acme/spacescale-api:latest", deploymentImageRef)
	require.EqualValues(t, 9090, deploymentRuntimePort)
	require.Nil(t, deploymentPublicURL)

	var microvmResourceType string
	var microvmResourceID *uuid.UUID
	var microvmWorkspaceID uuid.UUID
	var microvmNodeID *uuid.UUID
	var microvmRegion string
	var microvmVCPU int32
	var microvmRAMMB int64
	var microvmCPUMode string
	var microvmRootDiskMB int64
	var microvmVolumeMB int64
	var microvmStatus string
	var microvmError *string
	err = ts.pool.QueryRow(
		context.Background(),
		`SELECT workspace_id, resource_type, resource_id, node_id, region, vcpu, ram_mb, cpu_mode, root_disk_mb, volume_mb, status, error_message FROM microvms WHERE resource_type = 'deployment' AND resource_id = $1 ORDER BY created_at DESC LIMIT 1`,
		deploymentID,
	).Scan(&microvmWorkspaceID, &microvmResourceType, &microvmResourceID, &microvmNodeID, &microvmRegion, &microvmVCPU, &microvmRAMMB, &microvmCPUMode, &microvmRootDiskMB, &microvmVolumeMB, &microvmStatus, &microvmError)
	require.NoError(t, err)
	require.Equal(t, workspaceID, microvmWorkspaceID.String())
	require.Equal(t, "deployment", microvmResourceType)
	require.NotNil(t, microvmResourceID)
	require.Equal(t, deploymentID, *microvmResourceID)
	require.Nil(t, microvmNodeID)
	require.Equal(t, "ca-east", microvmRegion)
	require.EqualValues(t, 4, microvmVCPU)
	require.EqualValues(t, 4096, microvmRAMMB)
	require.Equal(t, "shared", microvmCPUMode)
	require.EqualValues(t, 5120, microvmRootDiskMB)
	require.Zero(t, microvmVolumeMB)
	require.Equal(t, "queued", microvmStatus)
	require.Nil(t, microvmError)

	var key string
	var encryptedValue string
	err = ts.pool.QueryRow(
		context.Background(),
		`SELECT key, value_encrypted FROM app_env_vars WHERE app_id = $1 ORDER BY created_at DESC LIMIT 1`,
		appID,
	).Scan(&key, &encryptedValue)
	require.NoError(t, err)
	require.Equal(t, "DATABASE_URL", key)
	require.NotEqual(t, "postgres://local", encryptedValue)
	require.True(t, strings.HasPrefix(encryptedValue, "v1:aesgcm:"))
}

// TestCreateAppDefaultsQueuedRuntimePort verifies runtime defaults remain
// consistent when callers omit runtimePort, while status remains queued.
func TestCreateAppDefaultsQueuedRuntimePort(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	identityKey := uniqueIdentityKey(t)
	syncAuthUserForTest(t, ts, identityKey)

	workspaceID := createWorkspaceForIdentity(t, ts, identityKey, fmt.Sprintf("workspace-%d", time.Now().UnixNano()))
	project := createProjectViaAPI(t, ts, identityKey, workspaceID, fmt.Sprintf("project-%d", time.Now().UnixNano()))

	body := []byte(`{"name":"worker","imageRef":"ghcr.io/acme/spacescale-worker:latest","compute":{"vcpu":2,"memoryMb":2048,"dedicated":false},"primaryRegion":"us-east"}`)
	resp, data := doRequest(
		t,
		ts,
		http.MethodPost,
		fmt.Sprintf("/v1/workspaces/%s/projects/%s/apps", workspaceID, project.ID),
		body,
		map[string]string{
			"Cookie":       authCookieForIdentityKey(t, identityKey),
			"Content-Type": "application/json",
		},
	)

	require.Equal(t, http.StatusCreated, resp.StatusCode, string(data))

	var out createAppResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.EqualValues(t, 1, out.TargetReplicas)
	require.Equal(t, "us-east", out.PrimaryRegion)
	require.Equal(t, "queued", out.Status)
	require.EqualValues(t, 8080, out.RuntimePort)
	require.False(t, out.IsPublic)
}

// TestCreateAppRejectsTooManyEnvVars verifies request validation rejects payloads
// with env var count above service limit.
func TestCreateAppRejectsTooManyEnvVars(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	identityKey := uniqueIdentityKey(t)
	syncAuthUserForTest(t, ts, identityKey)

	workspaceID := createWorkspaceForIdentity(t, ts, identityKey, fmt.Sprintf("workspace-%d", time.Now().UnixNano()))
	project := createProjectViaAPI(t, ts, identityKey, workspaceID, fmt.Sprintf("project-%d", time.Now().UnixNano()))

	envVars := make([]map[string]any, 0, 51)
	for i := range 51 {
		envVars = append(envVars, map[string]any{
			"key":      fmt.Sprintf("KEY_%d", i),
			"value":    "x",
			"isSecret": false,
		})
	}
	payload := map[string]any{
		"name":          "too-many-envs",
		"imageRef":      "ghcr.io/acme/spacescale-api:latest",
		"compute":       map[string]any{"vcpu": 4, "memoryMb": 8192, "dedicated": true},
		"primaryRegion": "eu-west",
		"envVars":       envVars,
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	resp, data := doRequest(
		t,
		ts,
		http.MethodPost,
		fmt.Sprintf("/v1/workspaces/%s/projects/%s/apps", workspaceID, project.ID),
		body,
		map[string]string{
			"Cookie":       authCookieForIdentityKey(t, identityKey),
			"Content-Type": "application/json",
		},
	)

	require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(data))
	var out errorResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.Equal(t, "invalid input", out.Error)
}

func TestCreateAppRequiresComputeAndPrimaryRegion(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	identityKey := uniqueIdentityKey(t)
	syncAuthUserForTest(t, ts, identityKey)

	workspaceID := createWorkspaceForIdentity(t, ts, identityKey, fmt.Sprintf("workspace-%d", time.Now().UnixNano()))
	project := createProjectViaAPI(t, ts, identityKey, workspaceID, fmt.Sprintf("project-%d", time.Now().UnixNano()))

	body := []byte(`{"name":"api","imageRef":"ghcr.io/acme/spacescale-api:latest"}`)
	resp, data := doRequest(
		t,
		ts,
		http.MethodPost,
		fmt.Sprintf("/v1/workspaces/%s/projects/%s/apps", workspaceID, project.ID),
		body,
		map[string]string{
			"Cookie":       authCookieForIdentityKey(t, identityKey),
			"Content-Type": "application/json",
		},
	)

	require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(data))
	var out errorResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.Equal(t, "invalid input", out.Error)
}

func TestListApps(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	identityKey := uniqueIdentityKey(t)
	syncAuthUserForTest(t, ts, identityKey)

	workspaceID := createWorkspaceForIdentity(t, ts, identityKey, fmt.Sprintf("workspace-%d", time.Now().UnixNano()))
	projectA := createProjectViaAPI(t, ts, identityKey, workspaceID, fmt.Sprintf("project-a-%d", time.Now().UnixNano()))
	projectB := createProjectViaAPI(t, ts, identityKey, workspaceID, fmt.Sprintf("project-b-%d", time.Now().UnixNano()))

	appOne := createAppViaAPI(t, ts, identityKey, workspaceID, projectA.ID, `{"name":"api","imageRef":"ghcr.io/acme/api:latest","compute":{"vcpu":2,"memoryMb":2048,"dedicated":false},"primaryRegion":"eu-central"}`)
	appTwo := createAppViaAPI(t, ts, identityKey, workspaceID, projectA.ID, `{"name":"worker","imageRef":"ghcr.io/acme/worker:latest","compute":{"vcpu":4,"memoryMb":4096,"dedicated":false},"primaryRegion":"eu-central"}`)
	_ = createAppViaAPI(t, ts, identityKey, workspaceID, projectB.ID, `{"name":"cron","imageRef":"ghcr.io/acme/cron:latest","compute":{"vcpu":4,"memoryMb":8192,"dedicated":true},"primaryRegion":"eu-central"}`)

	resp, data := doRequest(
		t,
		ts,
		http.MethodGet,
		fmt.Sprintf("/v1/workspaces/%s/projects/%s/apps", workspaceID, projectA.ID),
		nil,
		map[string]string{
			"Cookie": authCookieForIdentityKey(t, identityKey),
		},
	)

	require.Equal(t, http.StatusOK, resp.StatusCode, string(data))

	var out listAppsResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.Len(t, out.Apps, 2)
	require.Equal(t, appOne.ID, out.Apps[0].ID)
	require.Equal(t, projectA.ID, out.Apps[0].ProjectID)
	require.Equal(t, "api", out.Apps[0].Name)
	require.Equal(t, "eu-central", out.Apps[0].PrimaryRegion)
	require.Equal(t, "queued", out.Apps[0].Status)
	require.Equal(t, appTwo.ID, out.Apps[1].ID)
	require.Equal(t, projectA.ID, out.Apps[1].ProjectID)
	require.Equal(t, "worker", out.Apps[1].Name)
	require.Equal(t, "eu-central", out.Apps[1].PrimaryRegion)
	require.Equal(t, "queued", out.Apps[1].Status)
}

func TestListAppsRequiresOwnership(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	ownerIdentityKey := uniqueIdentityKey(t)
	otherIdentityKey := uniqueIdentityKey(t)
	syncAuthUserForTest(t, ts, ownerIdentityKey)
	syncAuthUserForTest(t, ts, otherIdentityKey)

	workspaceID := createWorkspaceForIdentity(t, ts, ownerIdentityKey, fmt.Sprintf("workspace-%d", time.Now().UnixNano()))
	project := createProjectViaAPI(t, ts, ownerIdentityKey, workspaceID, fmt.Sprintf("project-%d", time.Now().UnixNano()))
	_ = createAppViaAPI(t, ts, ownerIdentityKey, workspaceID, project.ID, `{"name":"api","imageRef":"ghcr.io/acme/api:latest","compute":{"vcpu":2,"memoryMb":2048,"dedicated":false},"primaryRegion":"eu-central"}`)

	resp, data := doRequest(
		t,
		ts,
		http.MethodGet,
		fmt.Sprintf("/v1/workspaces/%s/projects/%s/apps", workspaceID, project.ID),
		nil,
		map[string]string{
			"Cookie": authCookieForIdentityKey(t, otherIdentityKey),
		},
	)

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, string(data))
	var out errorResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.Equal(t, "unauthorized", out.Error)
}

func createAppViaAPI(t *testing.T, ts *testServer, identityKey, workspaceID, projectID, body string) createAppResponse {
	t.Helper()

	resp, data := doRequest(
		t,
		ts,
		http.MethodPost,
		fmt.Sprintf("/v1/workspaces/%s/projects/%s/apps", workspaceID, projectID),
		[]byte(body),
		map[string]string{
			"Cookie":       authCookieForIdentityKey(t, identityKey),
			"Content-Type": "application/json",
		},
	)

	require.Equal(t, http.StatusCreated, resp.StatusCode, string(data))

	var out createAppResponse
	require.NoError(t, json.Unmarshal(data, &out))
	return out
}
