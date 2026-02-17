// This file adds white-box tests for auth-user synchronization behavior in the
// service layer.
// The suite intentionally avoids a real database by using a small scripted DBTX
// test double that feeds sqlc query methods deterministic rows/errors.
// Coverage focus:
// - identity validation and input trimming rules
// - existing-profile preservation behavior across provider switches
// - database error propagation for both read and upsert stages
// - helper conversion behavior for nullable text fields

package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
	pgstore "github.com/t0gun/spacescale/internal/postgres/gen"
)

// syncQueryExpectation defines one expected QueryRow interaction against the
// sqlc-generated query layer.
// queryNameMatch is matched as a substring against the SQL text so tests remain
// resilient to harmless formatting changes in generated statements.
// assertArgs validates the exact argument payload forwarded by service logic.
type syncQueryExpectation struct {
	queryNameMatch string
	assertArgs     func(args ...interface{})
	row            pgx.Row
}

// scriptedSyncDB is a deterministic DBTX test double used by sync tests.
// It validates QueryRow call order and SQL intent and returns preconfigured rows.
type scriptedSyncDB struct {
	t            *testing.T
	expectations []syncQueryExpectation
	nextIndex    int
}

// newScriptedSyncDB creates a scripted DBTX double with ordered expectations.
func newScriptedSyncDB(t *testing.T, expectations []syncQueryExpectation) *scriptedSyncDB {
	t.Helper()
	return &scriptedSyncDB{
		t:            t,
		expectations: expectations,
	}
}

// QueryRow validates the next expected query and returns its scripted row.
func (db *scriptedSyncDB) QueryRow(_ context.Context, sql string, args ...interface{}) pgx.Row {
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

// Query is not expected in these tests and fails fast if called.
func (db *scriptedSyncDB) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	db.t.Helper()
	db.t.Fatalf("unexpected Query call in sync test")
	return nil, errors.New("unexpected Query call")
}

// Exec is not expected in these tests and fails fast if called.
func (db *scriptedSyncDB) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
	db.t.Helper()
	db.t.Fatalf("unexpected Exec call in sync test")
	return pgconn.CommandTag{}, errors.New("unexpected Exec call")
}

// assertDone verifies all scripted QueryRow expectations were consumed.
func (db *scriptedSyncDB) assertDone() {
	db.t.Helper()
	require.Equal(db.t, len(db.expectations), db.nextIndex, "not all expected QueryRow calls were consumed")
}

// scriptedRow is a minimal pgx.Row implementation used by tests.
// It either returns a preset error or scans a typed value list into destinations.
type scriptedRow struct {
	values []interface{}
	err    error
}

// Scan writes scripted values into destination pointers in sqlc scan order.
func (r scriptedRow) Scan(dest ...interface{}) error {
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

// asUserRowValues converts a sqlc user row into Scan-compatible value order.
func asUserRowValues(u pgstore.User) []interface{} {
	return []interface{}{
		u.ID,
		u.GithubID,
		u.Email,
		u.Name,
		u.AvatarUrl,
		u.CreatedAt,
		u.UpdatedAt,
	}
}

// buildSyncTestUser constructs deterministic sqlc user rows for sync tests.
func buildSyncTestUser(t *testing.T, githubID, email, name, avatarURL string) pgstore.User {
	t.Helper()

	now := time.Date(2026, 2, 17, 12, 0, 0, 0, time.UTC)
	return pgstore.User{
		ID:       mustUUID(t, "550e8400-e29b-41d4-a716-446655440000"),
		GithubID: githubID,
		Email: pgtype.Text{
			String: email,
			Valid:  strings.TrimSpace(email) != "",
		},
		Name: pgtype.Text{
			String: name,
			Valid:  strings.TrimSpace(name) != "",
		},
		AvatarUrl: pgtype.Text{
			String: avatarURL,
			Valid:  strings.TrimSpace(avatarURL) != "",
		},
		CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		UpdatedAt: pgtype.Timestamptz{Time: now.Add(5 * time.Minute), Valid: true},
	}
}

// TestSyncAuthUserRejectsEmptyIdentity verifies required identity validation.
// Service should fail fast before touching persistence when identity is blank.
func TestSyncAuthUserRejectsEmptyIdentity(t *testing.T) {
	svc := &ProjectService{}

	_, err := svc.SyncAuthUser(context.Background(), SyncAuthUserParams{
		IdentityKey: "   ",
	})

	require.ErrorIs(t, err, ErrInvalidInput)
}

// TestSyncAuthUserUpsertsNewUser verifies the happy path for first sign-in.
// It covers:
// - identity and profile field trimming
// - new-user flow when lookup returns pgx.ErrNoRows
// - nullable text conversion on upsert args
func TestSyncAuthUserUpsertsNewUser(t *testing.T) {
	row := buildSyncTestUser(t, "github:12345", "dev@example.com", "Dev", "https://example.com/a.png")

	db := newScriptedSyncDB(t, []syncQueryExpectation{
		{
			queryNameMatch: "GetUserByGithubID",
			assertArgs: func(args ...interface{}) {
				require.Len(t, args, 1)
				require.Equal(t, "github:12345", args[0])
			},
			row: scriptedRow{err: pgx.ErrNoRows},
		},
		{
			queryNameMatch: "UpsertUserByGithubID",
			assertArgs: func(args ...interface{}) {
				require.Len(t, args, 4)
				require.Equal(t, "github:12345", args[0])

				email, ok := args[1].(pgtype.Text)
				require.True(t, ok)
				require.Equal(t, pgtype.Text{String: "dev@example.com", Valid: true}, email)

				name, ok := args[2].(pgtype.Text)
				require.True(t, ok)
				require.Equal(t, pgtype.Text{String: "Dev", Valid: true}, name)

				avatar, ok := args[3].(pgtype.Text)
				require.True(t, ok)
				require.Equal(t, pgtype.Text{String: "https://example.com/a.png", Valid: true}, avatar)
			},
			row: scriptedRow{values: asUserRowValues(row)},
		},
	})
	svc := NewProjectService(pgstore.New(db))

	user, err := svc.SyncAuthUser(context.Background(), SyncAuthUserParams{
		IdentityKey: "  github:12345  ",
		Email:       "  dev@example.com  ",
		Name:        "  Dev  ",
		AvatarURL:   "  https://example.com/a.png  ",
	})

	require.NoError(t, err)
	require.Equal(t, "550e8400-e29b-41d4-a716-446655440000", user.ID)
	require.Equal(t, "github:12345", user.GithubID)
	require.Equal(t, "dev@example.com", user.Email)
	require.Equal(t, "Dev", user.Name)
	require.Equal(t, "https://example.com/a.png", user.AvatarURL)
	db.assertDone()
}

// TestSyncAuthUserPreservesExistingProfileValues verifies profile stickiness.
// Once profile fields exist in storage, provider data should not overwrite them
// on subsequent sign-ins.
func TestSyncAuthUserPreservesExistingProfileValues(t *testing.T) {
	existing := buildSyncTestUser(t, "github:12345", "stored@example.com", "Stored Name", "https://example.com/stored.png")
	updated := buildSyncTestUser(t, "github:12345", "stored@example.com", "Stored Name", "https://example.com/stored.png")

	db := newScriptedSyncDB(t, []syncQueryExpectation{
		{
			queryNameMatch: "GetUserByGithubID",
			assertArgs: func(args ...interface{}) {
				require.Len(t, args, 1)
				require.Equal(t, "github:12345", args[0])
			},
			row: scriptedRow{values: asUserRowValues(existing)},
		},
		{
			queryNameMatch: "UpsertUserByGithubID",
			assertArgs: func(args ...interface{}) {
				require.Len(t, args, 4)
				require.Equal(t, "github:12345", args[0])

				email, ok := args[1].(pgtype.Text)
				require.True(t, ok)
				require.Equal(t, pgtype.Text{String: "stored@example.com", Valid: true}, email)

				name, ok := args[2].(pgtype.Text)
				require.True(t, ok)
				require.Equal(t, pgtype.Text{String: "Stored Name", Valid: true}, name)

				avatar, ok := args[3].(pgtype.Text)
				require.True(t, ok)
				require.Equal(t, pgtype.Text{String: "https://example.com/stored.png", Valid: true}, avatar)
			},
			row: scriptedRow{values: asUserRowValues(updated)},
		},
	})
	svc := NewProjectService(pgstore.New(db))

	user, err := svc.SyncAuthUser(context.Background(), SyncAuthUserParams{
		IdentityKey: "github:12345",
		Email:       "new@example.com",
		Name:        "New Name",
		AvatarURL:   "https://example.com/new.png",
	})

	require.NoError(t, err)
	require.Equal(t, "stored@example.com", user.Email)
	require.Equal(t, "Stored Name", user.Name)
	require.Equal(t, "https://example.com/stored.png", user.AvatarURL)
	db.assertDone()
}

// TestSyncAuthUserReturnsLookupError verifies unexpected read errors bubble up.
func TestSyncAuthUserReturnsLookupError(t *testing.T) {
	lookupErr := errors.New("lookup failed")
	db := newScriptedSyncDB(t, []syncQueryExpectation{
		{
			queryNameMatch: "GetUserByGithubID",
			row:            scriptedRow{err: lookupErr},
		},
	})
	svc := NewProjectService(pgstore.New(db))

	_, err := svc.SyncAuthUser(context.Background(), SyncAuthUserParams{
		IdentityKey: "github:12345",
	})

	require.ErrorIs(t, err, lookupErr)
	db.assertDone()
}

// TestSyncAuthUserReturnsUpsertError verifies write failures bubble up.
func TestSyncAuthUserReturnsUpsertError(t *testing.T) {
	upsertErr := errors.New("upsert failed")
	db := newScriptedSyncDB(t, []syncQueryExpectation{
		{
			queryNameMatch: "GetUserByGithubID",
			row:            scriptedRow{err: pgx.ErrNoRows},
		},
		{
			queryNameMatch: "UpsertUserByGithubID",
			row:            scriptedRow{err: upsertErr},
		},
	})
	svc := NewProjectService(pgstore.New(db))

	_, err := svc.SyncAuthUser(context.Background(), SyncAuthUserParams{
		IdentityKey: "github:12345",
	})

	require.ErrorIs(t, err, upsertErr)
	db.assertDone()
}

// TestTextToPG verifies nullable text conversion rules used during persistence.
func TestTextToPG(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want pgtype.Text
	}{
		{
			name: "empty becomes null text",
			in:   "",
			want: pgtype.Text{Valid: false},
		},
		{
			name: "whitespace becomes null text",
			in:   "   ",
			want: pgtype.Text{Valid: false},
		},
		{
			name: "non-empty remains valid",
			in:   "dev@example.com",
			want: pgtype.Text{String: "dev@example.com", Valid: true},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, textToPG(tc.in))
		})
	}
}

// TestKeepExistingIfPresent verifies stored values win once populated.
func TestKeepExistingIfPresent(t *testing.T) {
	tests := []struct {
		name     string
		existing string
		incoming string
		want     string
	}{
		{
			name:     "keeps existing when present",
			existing: "stored@example.com",
			incoming: "incoming@example.com",
			want:     "stored@example.com",
		},
		{
			name:     "trims existing when present",
			existing: "  stored@example.com  ",
			incoming: "incoming@example.com",
			want:     "stored@example.com",
		},
		{
			name:     "uses incoming when existing empty",
			existing: "   ",
			incoming: "  incoming@example.com  ",
			want:     "incoming@example.com",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, keepExistingIfPresent(tc.existing, tc.incoming))
		})
	}
}
