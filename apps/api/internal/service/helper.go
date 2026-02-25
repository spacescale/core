// This file contains shared service-layer helpers reused across domain services.
// Centralizing these utilities keeps cross-service behavior consistent and makes
// common validation/normalization logic easier to evolve safely.

package service

import (
	"crypto/rand"
	"errors"
	"math/big"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	defaultRegion  = "global"
	maxSlugRetries = 8
	suffixLength   = 6
)

// parseUUID trims and parses UUID strings used by service workflows.
func parseUUID(raw string) (uuid.UUID, bool) {
	id, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

// uuidOrEmpty converts UUID values into API-safe optional strings.
func uuidOrEmpty(id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}
	return id.String()
}

// slugifyProjectName converts a raw project name into a stable URL slug.
// The transformation is intentionally strict and predictable:
// 1) trim surrounding whitespace and lowercase the input,
// 2) keep only letters and numbers while collapsing separator runs to one hyphen,
// 3) trim edge hyphens so the returned value is path-safe.
func slugifyProjectName(name string) string {
	// Normalize first so casing and accidental outer spaces never affect slug
	// uniqueness or readability.
	normalized := strings.ToLower(strings.TrimSpace(name))

	var b strings.Builder // build slug incrementally to avoid repeated string allocations.

	var prevHyphen bool // tracks whether the most recently written byte was '-' for separator collapsing.

	for _, r := range normalized {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			// Keep alphanumeric runes exactly as they are after normalization.
			b.WriteRune(r)
			prevHyphen = false
		default:
			// Treat every non-alphanumeric rune as a separator boundary.
			// Add one hyphen only when:
			// - the previous output character was not already a hyphen, and
			// - output is not empty (prevents leading hyphens).
			if !prevHyphen && b.Len() > 0 {
				b.WriteByte('-')
				prevHyphen = true
			}
		}
	}

	// Remove boundary hyphens that can appear when input begins or ends with
	// separators.
	return strings.Trim(b.String(), "-")
}

// randomSuffix returns a random lowercase alphanumeric suffix of fixed length.
// It is used only for slug collision retries so the user-visible base slug
// remains stable while each persistence attempt gets a fresh candidate.
func randomSuffix(n int) (string, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	if n <= 0 {
		return "", nil
	}

	var b strings.Builder
	b.Grow(n)
	max := big.NewInt(int64(len(alphabet)))

	for i := 0; i < n; i++ {
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b.WriteByte(alphabet[idx.Int64()])
	}

	return b.String(), nil
}

// isUniqueViolation reports whether an error is a PostgreSQL unique violation.
// The result is used to trigger conflict retries and normalize expected
// duplicate-write behavior across services.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}
