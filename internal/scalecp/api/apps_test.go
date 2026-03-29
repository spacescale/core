// This file verifies end-to-end behavior of create-app HTTP workflows.
//
// Scope:
// - Request/response contract for app creation.
// - Initial status behavior (queued).
// - Persistence side effects in deployments, machines, and app_env_vars tables.
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
	ID            string `json:"id"`
	ProjectID     string `json:"projectId"`
	Name          string `json:"name"`
	Slug          string `json:"slug"`
	Subdomain     string `json:"subdomain"`
	ImageRef      string `json:"imageRef"`
	Tier          string `json:"tier"`
	PrimaryRegion string `json:"primaryRegion"`
	RuntimePort   int32  `json:"runtimePort"`
	Status        string `json:"status"`
	IsPublic      bool   `json:"isPublic"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

// TestCreateAppCreatesQueuedDeployment verifies create-app writes app,
// deployment, and machine state, returns queued status, and stores env vars.
func TestCreateAppCreatesQueuedDeployment(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	identityKey := uniqueIdentityKey(t)
	syncAuthUserForTest(t, ts, identityKey)

	workspaceID := createWorkspaceForIdentity(t, ts, identityKey, fmt.Sprintf("workspace-%d", time.Now().UnixNano()))
	project := createProjectViaAPI(t, ts, identityKey, workspaceID, fmt.Sprintf("project-%d", time.Now().UnixNano()))

	body := []byte(`{"name":"api","imageRef":"ghcr.io/acme/spacescale-api:latest","tier":"growth","primaryRegion":"ca-east","runtimePort":9090,"isPublic":true,"envVars":[{"key":"database_url","value":"postgres://local","isSecret":true}]}`)
	resp, data := doRequest(
		t,
		ts,
		http.MethodPost,
		fmt.Sprintf("/v1/workspaces/%s/projects/%s/apps", workspaceID, project.ID),
		body,
		map[string]string{
			"Authorization": authHeaderForIdentityKey(t, identityKey),
			"Content-Type":  "application/json",
		},
	)

	require.Equal(t, http.StatusCreated, resp.StatusCode, string(data))

	var out createAppResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.NotEmpty(t, out.ID)
	require.Equal(t, project.ID, out.ProjectID)
	require.Equal(t, "api", out.Name)
	require.Equal(t, "growth", out.Tier)
	require.Equal(t, "ca-east", out.PrimaryRegion)
	require.Equal(t, "queued", out.Status)
	require.EqualValues(t, 9090, out.RuntimePort)
	require.NotEmpty(t, resp.Header.Get("Location"))

	appID, err := uuid.Parse(out.ID)
	require.NoError(t, err)

	var appStatus string
	err = ts.pool.QueryRow(context.Background(), `SELECT status FROM apps WHERE id = $1`, appID).Scan(&appStatus)
	require.NoError(t, err)
	require.Equal(t, "queued", appStatus)

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

	var machineDeploymentID uuid.UUID
	var machineNodeID *string
	var machineRegion string
	var machineTier string
	var machineStatus string
	var machineError *string
	err = ts.pool.QueryRow(
		context.Background(),
		`SELECT deployment_id, node_id, region, tier, status, error_message FROM machines WHERE app_id = $1 ORDER BY created_at DESC LIMIT 1`,
		appID,
	).Scan(&machineDeploymentID, &machineNodeID, &machineRegion, &machineTier, &machineStatus, &machineError)
	require.NoError(t, err)
	require.Equal(t, deploymentID, machineDeploymentID)
	require.Nil(t, machineNodeID)
	require.Equal(t, "ca-east", machineRegion)
	require.Equal(t, "growth", machineTier)
	require.Equal(t, "queued", machineStatus)
	require.Nil(t, machineError)

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

	body := []byte(`{"name":"worker","imageRef":"ghcr.io/acme/spacescale-worker:latest","tier":"starter","primaryRegion":"us-east"}`)
	resp, data := doRequest(
		t,
		ts,
		http.MethodPost,
		fmt.Sprintf("/v1/workspaces/%s/projects/%s/apps", workspaceID, project.ID),
		body,
		map[string]string{
			"Authorization": authHeaderForIdentityKey(t, identityKey),
			"Content-Type":  "application/json",
		},
	)

	require.Equal(t, http.StatusCreated, resp.StatusCode, string(data))

	var out createAppResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.Equal(t, "starter", out.Tier)
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
	for i := 0; i < 51; i++ {
		envVars = append(envVars, map[string]any{
			"key":      fmt.Sprintf("KEY_%d", i),
			"value":    "x",
			"isSecret": false,
		})
	}
	payload := map[string]any{
		"name":          "too-many-envs",
		"imageRef":      "ghcr.io/acme/spacescale-api:latest",
		"tier":          "scale",
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
			"Authorization": authHeaderForIdentityKey(t, identityKey),
			"Content-Type":  "application/json",
		},
	)

	require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(data))
	var out errorResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.Equal(t, "invalid input", out.Error)
}

func TestCreateAppRequiresTierAndPrimaryRegion(t *testing.T) {
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
			"Authorization": authHeaderForIdentityKey(t, identityKey),
			"Content-Type":  "application/json",
		},
	)

	require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(data))
	var out errorResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.Equal(t, "invalid input", out.Error)
}
