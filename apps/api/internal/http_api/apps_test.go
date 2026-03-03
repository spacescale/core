// This file verifies end-to-end behavior of create-app HTTP workflows.
//
// Scope:
// - Request/response contract for app creation.
// - Initial status behavior (queued).
// - Persistence side effects in deployments and app_env_vars tables.
//
// These are DB-backed integration tests by design so transport + service + SQL
// behavior are exercised together as one externally observable contract.

package http_api_test

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
	pgstore "github.com/t0gun/spacescale/internal/postgres/gen"
	"github.com/t0gun/spacescale/internal/service"
)

const (
	testLegacyEnvEncryptionKeyID = "legacy-v0"
	testLegacyEnvEncryptionKey   = "fedcba9876543210fedcba9876543210"
)

type createAppResponse struct {
	ID          string `json:"id"`
	ProjectID   string `json:"projectId"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Subdomain   string `json:"subdomain"`
	ImageRef    string `json:"imageRef"`
	RuntimePort int32  `json:"runtimePort"`
	Status      string `json:"status"`
	IsPublic    bool   `json:"isPublic"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

// TestCreateAppCreatesQueuedDeployment verifies create-app writes both app and
// initial deployment state, returns queued status, and stores encrypted env vars.
func TestCreateAppCreatesQueuedDeployment(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	identityKey := uniqueIdentityKey(t)
	syncAuthUserForTest(t, ts, identityKey)

	workspaceID := createWorkspaceForIdentity(t, ts, identityKey, fmt.Sprintf("workspace-%d", time.Now().UnixNano()))
	project := createProjectViaAPI(t, ts, identityKey, workspaceID, fmt.Sprintf("project-%d", time.Now().UnixNano()), "global")

	body := []byte(`{"name":"api","imageRef":"ghcr.io/acme/spacescale-api:latest","runtimePort":9090,"isPublic":true,"envVars":[{"key":"database_url","value":"postgres://local","isSecret":true}]}`)
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
	require.Equal(t, "queued", out.Status)
	require.EqualValues(t, 9090, out.RuntimePort)
	require.NotEmpty(t, resp.Header.Get("Location"))

	appID, err := uuid.Parse(out.ID)
	require.NoError(t, err)

	var appStatus string
	err = ts.pool.QueryRow(context.Background(), `SELECT status FROM apps WHERE id = $1`, appID).Scan(&appStatus)
	require.NoError(t, err)
	require.Equal(t, "queued", appStatus)

	var deploymentStatus string
	var deploymentImageRef string
	var deploymentRuntimePort int32
	var deploymentPublicURL *string
	err = ts.pool.QueryRow(
		context.Background(),
		`SELECT status, image_ref, runtime_port, public_url FROM deployments WHERE app_id = $1 ORDER BY created_at DESC LIMIT 1`,
		appID,
	).Scan(&deploymentStatus, &deploymentImageRef, &deploymentRuntimePort, &deploymentPublicURL)
	require.NoError(t, err)
	require.Equal(t, "queued", deploymentStatus)
	require.Equal(t, "ghcr.io/acme/spacescale-api:latest", deploymentImageRef)
	require.EqualValues(t, 9090, deploymentRuntimePort)
	require.Nil(t, deploymentPublicURL)

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
	project := createProjectViaAPI(t, ts, identityKey, workspaceID, fmt.Sprintf("project-%d", time.Now().UnixNano()), "global")

	body := []byte(`{"name":"worker","imageRef":"ghcr.io/acme/spacescale-worker:latest"}`)
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
	project := createProjectViaAPI(t, ts, identityKey, workspaceID, fmt.Sprintf("project-%d", time.Now().UnixNano()), "global")

	envVars := make([]map[string]any, 0, 51)
	for i := 0; i < 51; i++ {
		envVars = append(envVars, map[string]any{
			"key":      fmt.Sprintf("KEY_%d", i),
			"value":    "x",
			"isSecret": false,
		})
	}
	payload := map[string]any{
		"name":     "too-many-envs",
		"imageRef": "ghcr.io/acme/spacescale-api:latest",
		"envVars":  envVars,
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

// TestEnvReencryptWorkerMigratesLegacyCiphertext verifies background sweeps can
// decrypt legacy-key ciphertext and re-encrypt with the current active key.
func TestEnvReencryptWorkerMigratesLegacyCiphertext(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()

	identityKey := uniqueIdentityKey(t)
	syncAuthUserForTest(t, ts, identityKey)

	workspaceID := createWorkspaceForIdentity(t, ts, identityKey, fmt.Sprintf("workspace-%d", time.Now().UnixNano()))
	project := createProjectViaAPI(t, ts, identityKey, workspaceID, fmt.Sprintf("project-%d", time.Now().UnixNano()), "global")

	body := []byte(`{"name":"reencrypt","imageRef":"ghcr.io/acme/spacescale-api:latest","envVars":[{"key":"database_url","value":"postgres://legacy","isSecret":true}]}`)
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
	appID, err := uuid.Parse(out.ID)
	require.NoError(t, err)

	legacyCipher, err := service.NewEnvValueCipher(testLegacyEnvEncryptionKeyID, []byte(testLegacyEnvEncryptionKey))
	require.NoError(t, err)
	legacyEncrypted, err := legacyCipher.EncryptForStorage("postgres://legacy")
	require.NoError(t, err)

	_, err = ts.pool.Exec(
		context.Background(),
		`UPDATE app_env_vars SET value_encrypted = $1 WHERE app_id = $2 AND key = $3`,
		legacyEncrypted,
		appID,
		"DATABASE_URL",
	)
	require.NoError(t, err)

	keyring, err := service.NewEnvValueKeyring(testEnvEncryptionKeyID, map[string][]byte{
		testEnvEncryptionKeyID:       []byte(testEnvEncryptionKey),
		testLegacyEnvEncryptionKeyID: []byte(testLegacyEnvEncryptionKey),
	})
	require.NoError(t, err)

	worker, err := service.NewEnvValueReencryptWorker(service.EnvValueReencryptWorkerConfig{
		Pool:         ts.pool,
		Queries:      pgstore.New(ts.pool),
		Keyring:      keyring,
		ActiveKeyID:  testEnvEncryptionKeyID,
		LoadedKeyIDs: []string{testEnvEncryptionKeyID, testLegacyEnvEncryptionKeyID},
		BatchSize:    10,
		SweepPeriod:  20 * time.Millisecond,
	})
	require.NoError(t, err)

	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		worker.Run(workerCtx)
	}()

	deadline := time.Now().Add(3 * time.Second)
	for {
		var encryptedValue string
		err = ts.pool.QueryRow(
			context.Background(),
			`SELECT value_encrypted FROM app_env_vars WHERE app_id = $1 AND key = $2`,
			appID,
			"DATABASE_URL",
		).Scan(&encryptedValue)
		require.NoError(t, err)

		if strings.HasPrefix(encryptedValue, "v1:aesgcm:"+testEnvEncryptionKeyID+":") {
			plaintext, decErr := keyring.DecryptFromStorage(encryptedValue)
			require.NoError(t, decErr)
			require.Equal(t, "postgres://legacy", plaintext)
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("legacy ciphertext was not re-encrypted before deadline: %s", encryptedValue)
		}
		time.Sleep(25 * time.Millisecond)
	}

	workerCancel()
	select {
	case <-workerDone:
	case <-time.After(1 * time.Second):
		t.Fatal("reencryption worker did not stop after cancel")
	}
}
