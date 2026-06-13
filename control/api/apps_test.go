// This file verifies end-to-end behavior of workload HTTP workflows.
//
// Scope:
// - Request/response contracts for workload create and list endpoints.
// - Initial status behavior (queued).
// - Persistence side effects in deployments, microvms, and workload_env_vars tables.
//
// These are DB-backed integration tests by design so transport + service + SQL
// behavior are exercised together as one externally observable contract.

package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/spacescale/core/control/tenant"
	"github.com/stretchr/testify/require"
)

// TestCreateWorkloadCreatesQueuedDeployment verifies create-workload writes workload,
// deployment, and microvm state, returns queued status, and stores env vars.
func TestCreateWorkloadCreatesQueuedDeployment(t *testing.T) {
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
		fmt.Sprintf("/v1/workspaces/%s/projects/%s/workloads", workspaceID, project.ID),
		body,
		map[string]string{
			"Cookie":       authCookieForIdentityKey(t, identityKey),
			"Content-Type": "application/json",
		},
	)

	require.Equal(t, http.StatusCreated, resp.StatusCode, string(data))

	var out workloadResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.NotEmpty(t, out.ID)
	require.Equal(t, project.ID, out.ProjectID)
	require.Equal(t, "api", out.Name)
	require.EqualValues(t, 1, out.TargetReplicas)
	require.Equal(t, "ca-east", out.PrimaryRegion)
	require.Equal(t, "queued", out.Status)
	require.EqualValues(t, 9090, out.RuntimePort)
	require.NotEmpty(t, resp.Header.Get("Location"))

	workloadID, err := uuid.Parse(out.ID)
	require.NoError(t, err)

	var appStatus string
	var appTargetReplicas int32
	err = ts.pool.QueryRow(context.Background(), `SELECT status, target_replicas FROM workloads WHERE id = $1`, workloadID).Scan(&appStatus, &appTargetReplicas)
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
		`SELECT id, status, image_ref, runtime_port, public_url FROM deployments WHERE workload_id = $1 ORDER BY created_at DESC LIMIT 1`,
		workloadID,
	).Scan(&deploymentID, &deploymentStatus, &deploymentImageRef, &deploymentRuntimePort, &deploymentPublicURL)
	require.NoError(t, err)
	require.Equal(t, "queued", deploymentStatus)
	require.Equal(t, "ghcr.io/acme/spacescale-api:latest", deploymentImageRef)
	require.EqualValues(t, 9090, deploymentRuntimePort)
	require.Nil(t, deploymentPublicURL)

	var microvmResourceType string
	var microvmResourceID *uuid.UUID
	var microvmWorkspaceID uuid.UUID
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
		`SELECT workspace_id, resource_type, resource_id, region, vcpu, ram_mb, cpu_mode, root_disk_mb, volume_mb, status, error_message FROM microvms WHERE resource_type = 'deployment' AND resource_id = $1 ORDER BY created_at DESC LIMIT 1`,
		deploymentID,
	).Scan(&microvmWorkspaceID, &microvmResourceType, &microvmResourceID, &microvmRegion, &microvmVCPU, &microvmRAMMB, &microvmCPUMode, &microvmRootDiskMB, &microvmVolumeMB, &microvmStatus, &microvmError)
	require.NoError(t, err)
	require.Equal(t, workspaceID, microvmWorkspaceID.String())
	require.Equal(t, "deployment", microvmResourceType)
	require.NotNil(t, microvmResourceID)
	require.Equal(t, deploymentID, *microvmResourceID)
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
		`SELECT key, value_encrypted FROM workload_env_vars WHERE workload_id = $1 ORDER BY created_at DESC LIMIT 1`,
		workloadID,
	).Scan(&key, &encryptedValue)
	require.NoError(t, err)
	require.Equal(t, "DATABASE_URL", key)
	require.NotEqual(t, "postgres://local", encryptedValue)
	require.True(t, strings.HasPrefix(encryptedValue, "v1:xchacha20poly1305:"))
}

// TestCreateWorkloadDefaultsQueuedRuntimePort verifies runtime defaults remain
// consistent when callers omit runtimePort, while status remains queued.
func TestCreateWorkloadDefaultsQueuedRuntimePort(t *testing.T) {
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
		fmt.Sprintf("/v1/workspaces/%s/projects/%s/workloads", workspaceID, project.ID),
		body,
		map[string]string{
			"Cookie":       authCookieForIdentityKey(t, identityKey),
			"Content-Type": "application/json",
		},
	)

	require.Equal(t, http.StatusCreated, resp.StatusCode, string(data))

	var out workloadResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.EqualValues(t, 1, out.TargetReplicas)
	require.Equal(t, "us-east", out.PrimaryRegion)
	require.Equal(t, "queued", out.Status)
	require.EqualValues(t, 8080, out.RuntimePort)
	require.False(t, out.IsPublic)
}

// TestCreateWorkloadRejectsTooManyEnvVars verifies request validation rejects payloads
// with env var count above service limit.
func TestCreateWorkloadRejectsTooManyEnvVars(t *testing.T) {
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
		fmt.Sprintf("/v1/workspaces/%s/projects/%s/workloads", workspaceID, project.ID),
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

func TestCreateWorkloadRequiresComputeAndPrimaryRegion(t *testing.T) {
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
		fmt.Sprintf("/v1/workspaces/%s/projects/%s/workloads", workspaceID, project.ID),
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

func TestResolveCreateWorkloadAfterDispatch(t *testing.T) {
	t.Run("uses refreshed workload when available", func(t *testing.T) {
		current := tenant.Workload{ID: "workload-1", Status: "queued"}
		refreshed := tenant.Workload{ID: "workload-1", Status: "failed"}

		resolved := resolveCreateWorkloadAfterDispatch(current, errors.New("dispatch failed"), refreshed, nil)

		require.Equal(t, refreshed, resolved)
	})

	t.Run("marks deploying when dispatch succeeded and refresh failed", func(t *testing.T) {
		current := tenant.Workload{ID: "workload-1", Status: "queued"}

		resolved := resolveCreateWorkloadAfterDispatch(current, nil, tenant.Workload{}, errors.New("refresh failed"))

		require.Equal(t, "deploying", resolved.Status)
	})

	t.Run("keeps current status when dispatch and refresh both fail", func(t *testing.T) {
		current := tenant.Workload{ID: "workload-1", Status: "queued"}

		resolved := resolveCreateWorkloadAfterDispatch(current, errors.New("dispatch failed"), tenant.Workload{}, errors.New("refresh failed"))

		require.Equal(t, current, resolved)
	})
}

func TestNewCreateWorkloadDispatchContext(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	cancelParent()

	ctx, cancel := newCreateWorkloadDispatchContext(parent)
	defer cancel()

	require.NoError(t, ctx.Err())

	select {
	case <-ctx.Done():
		require.Fail(t, "dispatch context should ignore parent cancellation")
	case <-time.After(10 * time.Millisecond):
	}
}

func TestListWorkloads(t *testing.T) {
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
		fmt.Sprintf("/v1/workspaces/%s/projects/%s/workloads", workspaceID, projectA.ID),
		nil,
		map[string]string{
			"Cookie": authCookieForIdentityKey(t, identityKey),
		},
	)

	require.Equal(t, http.StatusOK, resp.StatusCode, string(data))

	var out listWorkloadsResponse
	require.NoError(t, json.Unmarshal(data, &out))
	require.Len(t, out.Workloads, 2)
	require.Equal(t, appOne.ID, out.Workloads[0].ID)
	require.Equal(t, projectA.ID, out.Workloads[0].ProjectID)
	require.Equal(t, "api", out.Workloads[0].Name)
	require.Equal(t, "eu-central", out.Workloads[0].PrimaryRegion)
	require.Equal(t, "queued", out.Workloads[0].Status)
	require.Equal(t, appTwo.ID, out.Workloads[1].ID)
	require.Equal(t, projectA.ID, out.Workloads[1].ProjectID)
	require.Equal(t, "worker", out.Workloads[1].Name)
	require.Equal(t, "eu-central", out.Workloads[1].PrimaryRegion)
	require.Equal(t, "queued", out.Workloads[1].Status)
}

func TestListWorkloadsRequiresOwnership(t *testing.T) {
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
		fmt.Sprintf("/v1/workspaces/%s/projects/%s/workloads", workspaceID, project.ID),
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

func createAppViaAPI(t *testing.T, ts *testServer, identityKey, workspaceID, projectID, body string) workloadResponse {
	t.Helper()

	resp, data := doRequest(
		t,
		ts,
		http.MethodPost,
		fmt.Sprintf("/v1/workspaces/%s/projects/%s/workloads", workspaceID, projectID),
		[]byte(body),
		map[string]string{
			"Cookie":       authCookieForIdentityKey(t, identityKey),
			"Content-Type": "application/json",
		},
	)

	require.Equal(t, http.StatusCreated, resp.StatusCode, string(data))

	var out workloadResponse
	require.NoError(t, json.Unmarshal(data, &out))
	return out
}
