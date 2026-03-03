// This file defines canonical API runtime configuration models.
// It keeps auth/rate-limit/log-privacy/internal-auth settings in one package so
// startup loading and HTTP transport wiring share a single contract.

package config

import (
	"errors"
	"strings"
	"time"
)

const (
	defaultUserRateLimitRequests             = 100
	defaultUserRateLimitWindow               = time.Minute
	defaultInternalGlobalRateLimitRequests   = 12000
	defaultInternalGlobalRateLimitWindow     = time.Minute
	defaultInternalIdentityRateLimitRequests = 30
	defaultInternalIdentityRateLimitWindow   = time.Minute

	defaultUserAgentLogMode    = UserAgentLogModeHash
	defaultUserAgentMaxLength  = 100
	defaultPanicValueMaxLength = 200
	defaultIncludeStackTrace   = false
)

// AuthConfig defines runtime settings used to verify incoming BFF-issued JWTs.
type AuthConfig struct {
	JWTSecret string
	Issuer    string
	Audience  string
}

// DefaultInternalGlobalRateLimitConfig returns defaults for global internal
// route circuit-breaker behavior.
func DefaultInternalGlobalRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		Requests: defaultInternalGlobalRateLimitRequests,
		Window:   defaultInternalGlobalRateLimitWindow,
	}
}

// Validate verifies required auth verification settings are present.
func (c AuthConfig) Validate() error {
	if strings.TrimSpace(c.JWTSecret) == "" {
		return errors.New("auth config JWTSecret is required")
	}
	if strings.TrimSpace(c.Issuer) == "" {
		return errors.New("auth config Issuer is required")
	}
	if strings.TrimSpace(c.Audience) == "" {
		return errors.New("auth config Audience is required")
	}
	return nil
}

// RateLimitConfig describes per-authenticated-user API rate-limiting behavior.
type RateLimitConfig struct {
	Requests int
	Window   time.Duration
}

// DefaultRateLimitConfig returns default per-user rate-limit settings.
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		Requests: defaultUserRateLimitRequests,
		Window:   defaultUserRateLimitWindow,
	}
}

// DefaultInternalIdentityRateLimitConfig returns defaults for per-identity
// limiting on internal auth-sync requests.
func DefaultInternalIdentityRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		Requests: defaultInternalIdentityRateLimitRequests,
		Window:   defaultInternalIdentityRateLimitWindow,
	}
}

func (c RateLimitConfig) normalizedWith(defaults RateLimitConfig) RateLimitConfig {
	if c.Requests <= 0 {
		c.Requests = defaults.Requests
	}
	if c.Window <= 0 {
		c.Window = defaults.Window
	}
	return c
}

// Normalized returns safe runtime limiter values when fields are invalid.
func (c RateLimitConfig) Normalized() RateLimitConfig {
	return c.normalizedWith(DefaultRateLimitConfig())
}

// UserAgentLogMode controls how user-agent data is emitted in logs.
type UserAgentLogMode string

const (
	UserAgentLogModeHash     UserAgentLogMode = "hash"
	UserAgentLogModeTruncate UserAgentLogMode = "truncate"
	UserAgentLogModeOff      UserAgentLogMode = "off"
)

// LogPrivacyConfig controls user-agent representation and panic-log redaction.
type LogPrivacyConfig struct {
	UserAgentMode       UserAgentLogMode
	UserAgentHashSecret string
	UserAgentMaxLen     int
	PanicValueMaxLen    int
	IncludeStackTrace   bool
}

// EnvEncryptionConfig defines key material used to encrypt app env vars.
type EnvEncryptionConfig struct {
	KeyID string
	Key   []byte
}

// DefaultLogPrivacyConfig returns package default privacy settings.
func DefaultLogPrivacyConfig() LogPrivacyConfig {
	return LogPrivacyConfig{
		UserAgentMode:     defaultUserAgentLogMode,
		UserAgentMaxLen:   defaultUserAgentMaxLength,
		PanicValueMaxLen:  defaultPanicValueMaxLength,
		IncludeStackTrace: defaultIncludeStackTrace,
	}
}

// Normalized applies safe fallbacks for unknown/invalid runtime values.
func (c LogPrivacyConfig) Normalized() LogPrivacyConfig {
	switch c.UserAgentMode {
	case UserAgentLogModeHash, UserAgentLogModeTruncate, UserAgentLogModeOff:
	default:
		c.UserAgentMode = defaultUserAgentLogMode
	}

	if c.UserAgentMaxLen <= 0 {
		c.UserAgentMaxLen = defaultUserAgentMaxLength
	}
	if c.PanicValueMaxLen <= 0 {
		c.PanicValueMaxLen = defaultPanicValueMaxLength
	}

	return c
}

// APIConfig groups runtime behavior settings for API middleware and internal
// endpoint protection.
type APIConfig struct {
	Auth                      AuthConfig
	RateLimit                 RateLimitConfig
	InternalGlobalRateLimit   RateLimitConfig
	InternalIdentityRateLimit RateLimitConfig
	LogPrivacy                LogPrivacyConfig
	InternalAuthSecret        string
	EnvEncryption             EnvEncryptionConfig
}

// Normalized returns a safe runtime API config for server wiring.
func (c APIConfig) Normalized() APIConfig {
	c.RateLimit = c.RateLimit.normalizedWith(DefaultRateLimitConfig())
	c.InternalGlobalRateLimit = c.InternalGlobalRateLimit.normalizedWith(DefaultInternalGlobalRateLimitConfig())
	c.InternalIdentityRateLimit = c.InternalIdentityRateLimit.normalizedWith(DefaultInternalIdentityRateLimitConfig())
	c.LogPrivacy = c.LogPrivacy.Normalized()
	c.InternalAuthSecret = strings.TrimSpace(c.InternalAuthSecret)
	c.EnvEncryption.KeyID = strings.TrimSpace(c.EnvEncryption.KeyID)
	return c
}
