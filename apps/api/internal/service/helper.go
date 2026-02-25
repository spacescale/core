// This file contains shared service-layer helpers reused across domain services.
// Centralizing these utilities keeps cross-service behavior consistent and makes
// common validation/normalization logic easier to evolve safely.

package service

import (
	"crypto/rand"
	"errors"
	"math/big"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	defaultRegion        = "global"
	maxSlugRetries       = 8
	suffixLength         = 6
	projectNameMaxLength = 120
	projectRegionMaxLen  = 32
	projectSlugMaxLength = 63
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
// The resulting slug is constrained to lowercase ASCII DNS-label-safe bytes:
// [a-z0-9-], with collapsed separators, no edge hyphens, and max length 63.
func slugifyProjectName(name string) string {
	normalized := strings.ToLower(strings.TrimSpace(name))

	var b strings.Builder
	b.Grow(len(normalized))
	var prevHyphen bool

	for i := 0; i < len(normalized); i++ {
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

// normalizeProjectName trims and validates the display name length.
func normalizeProjectName(raw string) (string, bool) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", false
	}
	if utf8.RuneCountInString(name) > projectNameMaxLength {
		return "", false
	}
	return name, true
}

// normalizeProjectRegion trims, lowercases, and validates region format.
// Regions are restricted to lowercase ASCII DNS-label-safe bytes for stability.
func normalizeProjectRegion(raw string) (string, bool) {
	region := strings.ToLower(strings.TrimSpace(raw))
	if region == "" {
		region = defaultRegion
	}
	if len(region) == 0 || len(region) > projectRegionMaxLen {
		return "", false
	}
	if region[0] == '-' || region[len(region)-1] == '-' {
		return "", false
	}
	for i := 0; i < len(region); i++ {
		ch := region[i]
		if ch == '-' {
			continue
		}
		if !isASCIIAlphaNum(ch) {
			return "", false
		}
	}
	return region, true
}

// slugWithSuffix appends a random suffix while keeping total slug length bounded.
func slugWithSuffix(baseSlug, suffix string) string {
	trimmedBase := strings.Trim(baseSlug, "-")
	trimmedSuffix := strings.Trim(suffix, "-")
	if trimmedSuffix == "" {
		if len(trimmedBase) <= projectSlugMaxLength {
			return trimmedBase
		}
		return strings.Trim(trimmedBase[:projectSlugMaxLength], "-")
	}

	maxBaseLen := projectSlugMaxLength - len(trimmedSuffix) - 1
	if maxBaseLen <= 0 {
		if len(trimmedSuffix) <= projectSlugMaxLength {
			return trimmedSuffix
		}
		return trimmedSuffix[:projectSlugMaxLength]
	}

	if len(trimmedBase) > maxBaseLen {
		trimmedBase = strings.Trim(trimmedBase[:maxBaseLen], "-")
	}
	if trimmedBase == "" {
		return trimmedSuffix
	}
	return trimmedBase + "-" + trimmedSuffix
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
