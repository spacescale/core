// This file adds focused transport-layer tests for the internal auth-sync
// handler used by the web authentication callback.
// The tests are intentionally handler-level and deterministic; they avoid a
// real database by wiring the service to a scripted DBTX test double.
// Coverage focus:
// - malformed JSON and required-field validation at handler boundary
// - service error mapping to HTTP status code contract
// - successful response payload serialization

package http_api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
	pgstore "github.com/t0gun/spacescale/internal/postgres/gen"
	"github.com/t0gun/spacescale/internal/service"
)

// authSyncQueryExpectation defines one expected sqlc QueryRow call.
// queryNameMatch is checked as a SQL substring so harmless SQL formatting
// differences in generated code do not break test intent.
type authSyncQueryExpectation struct {
	queryNameMatch string
	assertArgs     func(args ...interface{})
	row            pgx.Row
}

// scriptedAuthSyncDB is a deterministic DBTX test double used by auth-sync
// handler tests.
// It validates expected query order and returns scripted rows/errors.
type scriptedAuthSyncDB struct {
	t            *testing.T
	expectations []authSyncQueryExpectation
	nextIndex    int
}

// newScriptedAuthSyncDB constructs a scripted DB with ordered QueryRow checks.
func newScriptedAuthSyncDB(t *testing.T, expectations []authSyncQueryExpectation) *scriptedAuthSyncDB {
	t.Helper()
	return &scriptedAuthSyncDB{
		t:            t,
		expectations: expectations,
	}
}

// QueryRow validates and serves the next scripted query result.
func (db *scriptedAuthSyncDB) QueryRow(_ context.Context, sql string, args ...interface{}) pgx.Row {
	db.t.Helper()

	if db.nextIndex >= len(db.expectations) {
		db.t.Fatalf("unexpected QueryRow call: sql=%q args=%v", sql, args)
	}

	exp := db.expectations[db.nextIndex]
	db.nextIndex++

	require.Contains(db.t, sql, exp.queryNameMatch)
	if exp.assertArgs != nil {
		exp.assertArgs(args...)
	}
	return exp.row
}

// Query is not expected in auth-sync handler tests and fails fast.
func (db *scriptedAuthSyncDB) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	db.t.Helper()
	db.t.Fatalf("unexpected Query call in auth-sync handler test")
	return nil, errors.New("unexpected Query call")
}

// Exec is not expected in auth-sync handler tests and fails fast.
func (db *scriptedAuthSyncDB) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
	db.t.Helper()
	db.t.Fatalf("unexpected Exec call in auth-sync handler test")
	return pgconn.CommandTag{}, errors.New("unexpected Exec call")
}

// assertDone ensures all scripted expectations were consumed.
func (db *scriptedAuthSyncDB) assertDone() {
	db.t.Helper()
	require.Equal(db.t, len(db.expectations), db.nextIndex, "not all expected QueryRow calls were consumed")
}

// scriptedAuthSyncRow is a tiny pgx.Row implementation used in these tests.
// It either returns a predefined error or scans a typed value slice.
type scriptedAuthSyncRow struct {
	values []interface{}
	err    error
}

// Scan writes scripted values into destination pointers using sqlc scan order.
func (r scriptedAuthSyncRow) Scan(dest ...interface{}) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != len(r.values) {
		return fmt.Errorf("scan destination/value length mismatch: dest=%d values=%d", len(dest), len(r.values))
	}

	for i := range dest {
		switch d := dest[i].(type) {
		case *pgtype.UUID:
			v, ok := r.values[i].(pgtype.UUID)
			if !ok {
				return fmt.Errorf("value %d type mismatch: got %T want pgtype.UUID", i, r.values[i])
			}
			*d = v
		case *string:
			v, ok := r.values[i].(string)
			if !ok {
				return fmt.Errorf("value %d type mismatch: got %T want string", i, r.values[i])
			}
			*d = v
		case *pgtype.Text:
			v, ok := r.values[i].(pgtype.Text)
			if !ok {
				return fmt.Errorf("value %d type mismatch: got %T want pgtype.Text", i, r.values[i])
			}
			*d = v
		case *bool:
			v, ok := r.values[i].(bool)
			if !ok {
				return fmt.Errorf("value %d type mismatch: got %T want bool", i, r.values[i])
			}
			*d = v
		case *pgtype.Timestamptz:
			v, ok := r.values[i].(pgtype.Timestamptz)
			if !ok {
				return fmt.Errorf("value %d type mismatch: got %T want pgtype.Timestamptz", i, r.values[i])
			}
			*d = v
		default:
			return fmt.Errorf("unsupported scan destination type %T", dest[i])
		}
	}
	return nil
}

// asAuthSyncUserRowValues maps a sqlc user row to Scan value order.
func asAuthSyncUserRowValues(u pgstore.User) []interface{} {
	return []interface{}{
		u.ID,
		u.GithubID,
		u.Email,
		u.Name,
		u.AvatarUrl,
		u.OnboardingCompleted,
		u.CreatedAt,
		u.UpdatedAt,
	}
}

// buildAuthSyncTestUser returns a deterministic persisted user row for tests.
func buildAuthSyncTestUser(t *testing.T, githubID string) pgstore.User {
	t.Helper()

	var id pgtype.UUID
	require.NoError(t, id.Scan("550e8400-e29b-41d4-a716-446655440000"))
	now := time.Date(2026, 2, 17, 12, 0, 0, 0, time.UTC)

	return pgstore.User{
		ID:       id,
		GithubID: githubID,
		Email:    pgtype.Text{String: "dev@example.com", Valid: true},
		Name:     pgtype.Text{String: "Dev", Valid: true},
		AvatarUrl: pgtype.Text{
			String: "https://example.com/avatar.png",
			Valid:  true,
		},
		OnboardingCompleted: true,
		CreatedAt:           pgtype.Timestamptz{Time: now, Valid: true},
		UpdatedAt:           pgtype.Timestamptz{Time: now.Add(5 * time.Minute), Valid: true},
	}
}

// TestHandleSyncAuthUserInvalidJSON verifies malformed payload handling.
func TestHandleSyncAuthUserInvalidJSON(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/v0/internal/auth-sync", strings.NewReader("{"))
	rr := httptest.NewRecorder()

	s.handleSyncAuthUser(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	var out errResp
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &out))
	require.Equal(t, "invalid json", out.Error)
}

// TestHandleSyncAuthUserRejectsEmptyIdentity verifies required field validation.
func TestHandleSyncAuthUserRejectsEmptyIdentity(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/v0/internal/auth-sync", strings.NewReader(`{"identityKey":"   "}`))
	rr := httptest.NewRecorder()

	s.handleSyncAuthUser(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	var out errResp
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &out))
	require.Equal(t, "invalid input", out.Error)
}

// TestHandleSyncAuthUserMapsServiceError verifies generic internal error mapping.
func TestHandleSyncAuthUserMapsServiceError(t *testing.T) {
	lookupErr := errors.New("db lookup failed")
	db := newScriptedAuthSyncDB(t, []authSyncQueryExpectation{
		{
			queryNameMatch: "GetUserByGithubID",
			row:            scriptedAuthSyncRow{err: lookupErr},
		},
	})
	s := &Server{
		svc: service.NewProjectService(pgstore.New(db)),
	}

	req := httptest.NewRequest(http.MethodPost, "/v0/internal/auth-sync", strings.NewReader(`{"identityKey":"github:12345"}`))
	rr := httptest.NewRecorder()

	s.handleSyncAuthUser(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	var out errResp
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &out))
	require.Equal(t, "internal error", out.Error)
	db.assertDone()
}

// TestHandleSyncAuthUserSuccess verifies successful response shape and status.
func TestHandleSyncAuthUserSuccess(t *testing.T) {
	userRow := buildAuthSyncTestUser(t, "github:12345")
	db := newScriptedAuthSyncDB(t, []authSyncQueryExpectation{
		{
			queryNameMatch: "GetUserByGithubID",
			row:            scriptedAuthSyncRow{err: pgx.ErrNoRows},
		},
		{
			queryNameMatch: "UpsertUserByGithubID",
			assertArgs: func(args ...interface{}) {
				require.Len(t, args, 4)
				require.Equal(t, "github:12345", args[0])
			},
			row: scriptedAuthSyncRow{values: asAuthSyncUserRowValues(userRow)},
		},
	})
	s := &Server{
		svc: service.NewProjectService(pgstore.New(db)),
	}

	req := httptest.NewRequest(http.MethodPost, "/v0/internal/auth-sync", strings.NewReader(`{"identityKey":"github:12345","email":"dev@example.com","name":"Dev","avatarUrl":"https://example.com/avatar.png"}`))
	rr := httptest.NewRecorder()

	s.handleSyncAuthUser(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var out syncAuthUserResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &out))
	require.Equal(t, "550e8400-e29b-41d4-a716-446655440000", out.ID)
	require.True(t, out.OnboardingCompleted)
	db.assertDone()
}
