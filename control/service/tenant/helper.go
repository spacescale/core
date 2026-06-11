// This file contains shared service-layer helpers reused across domain services.
// Centralizing these utilities keeps cross-service behavior consistent and makes
// common validation/normalization logic easier to evolve safely.

package tenant

import (
	"crypto/rand"
	"errors"
	"math/big"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	maxSlugRetries       = 8
	suffixLength         = 6
	projectSlugMaxLength = 63
)

// parseUUID parses UUID strings used by service workflows.
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
// The resulting slug is constrained to lowercase ASCII DNS-label-safe bytes:
// [a-z0-9-], with collapsed separators, no edge hyphens, and max length 63.
func slugifyProjectName(name string) string {
	normalized := strings.ToLower(name)

	var b strings.Builder
	b.Grow(len(normalized))
	var prevHyphen bool

	for i := range len(normalized) {
		ch := normalized[i]
		if isASCIIAlphaNum(ch) {
			b.WriteByte(ch)
			prevHyphen = false
			continue
		}
		if !prevHyphen && b.Len() > 0 {
			b.WriteByte('-')
			prevHyphen = true
		}
	}

	slug := strings.Trim(b.String(), "-")
	if len(slug) <= projectSlugMaxLength {
		return slug
	}

	return strings.Trim(slug[:projectSlugMaxLength], "-")
}

// normalizeWorkspaceName trims the workspace display name.
func normalizeWorkspaceName(raw string) (string, bool) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", false
	}
	return name, true
}

// normalizeProjectName trims the project display name.
func normalizeProjectName(raw string) (string, bool) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", false
	}
	return name, true
}

// slugWithSuffix appends a random suffix while keeping total slug length bounded.
func slugWithSuffix(baseSlug, suffix string) string {
	maxBaseLen := projectSlugMaxLength - len(suffix) - 1
	if maxBaseLen <= 0 {
		if len(suffix) <= projectSlugMaxLength {
			return suffix
		}
		return suffix[:projectSlugMaxLength]
	}

	if len(baseSlug) > maxBaseLen {
		baseSlug = strings.Trim(baseSlug[:maxBaseLen], "-")
	}
	if baseSlug == "" {
		return suffix
	}
	return baseSlug + "-" + suffix
}

func isASCIIAlphaNum(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9')
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
	alphabetLen := big.NewInt(int64(len(alphabet)))

	for range n {
		idx, err := rand.Int(rand.Reader, alphabetLen)
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
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		return pgErr.Code == "23505"
	}
	return false
}
