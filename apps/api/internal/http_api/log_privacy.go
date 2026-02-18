// This file centralizes request-log privacy configuration and helpers.
// It defines user-agent logging modes, panic redaction defaults, and helper
// functions shared by access/auth/panic logging paths.

package http_api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"runtime/debug"
	"strings"
)

// UserAgentLogMode controls how user-agent data is emitted in logs.
type UserAgentLogMode string

const (
	UserAgentLogModeHash     UserAgentLogMode = "hash"     // logs user-agent as a non-reversible hash.
	UserAgentLogModeTruncate UserAgentLogMode = "truncate" // logs user-agent as a truncated raw string.
	UserAgentLogModeOff      UserAgentLogMode = "off"      // omits user-agent fields from logs.
)

const (
	defaultUserAgentLogMode    = UserAgentLogModeHash // fallback mode when config is missing or unknown.
	defaultUserAgentMaxLength  = 100                  // fallback length for truncate mode.
	defaultPanicValueMaxLength = 200                  // fallback panic_value length cap for log safety.
	defaultIncludeStackTrace   = false                // stack traces are disabled by default for production safety.
)

// LogPrivacyConfig controls user-agent representation and panic-log redaction.
type LogPrivacyConfig struct {
	UserAgentMode       UserAgentLogMode
	UserAgentHashSecret string
	UserAgentMaxLen     int
	PanicValueMaxLen    int
	IncludeStackTrace   bool
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

// normalized applies safe fallbacks for unknown/invalid runtime config values.
func (c LogPrivacyConfig) normalized() LogPrivacyConfig {
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

// userAgentLogAttr returns a log key/value for user-agent according to policy.
// It returns ok=false when user-agent logging is disabled or input is empty.
func userAgentLogAttr(rawUserAgent string, cfg LogPrivacyConfig) (string, string, bool) {
	cfg = cfg.normalized()
	ua := strings.TrimSpace(rawUserAgent)
	if ua == "" {
		return "", "", false
	}

	switch cfg.UserAgentMode {
	case UserAgentLogModeOff:
		return "", "", false

	case UserAgentLogModeTruncate:
		return "user_agent", truncateLogString(ua, cfg.UserAgentMaxLen), true

	case UserAgentLogModeHash:
		secret := strings.TrimSpace(cfg.UserAgentHashSecret)
		if secret == "" {
			return "", "", false
		}
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write([]byte(ua))
		return "user_agent_hash", hex.EncodeToString(mac.Sum(nil)), true

	default:
		return "", "", false
	}
}

// truncateLogString returns at most maxLen runes from input.
func truncateLogString(input string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	runes := []rune(input)
	if len(runes) <= maxLen {
		return input
	}
	return string(runes[:maxLen])
}

// panicValueLogValue stringifies and truncates panic value for safer logging.
func panicValueLogValue(recovered any, cfg LogPrivacyConfig) string {
	cfg = cfg.normalized()
	return truncateLogString(fmt.Sprint(recovered), cfg.PanicValueMaxLen)
}

// panicStackTraceLogAttr returns stack-trace log field when enabled by config.
func panicStackTraceLogAttr(cfg LogPrivacyConfig) (string, string, bool) {
	cfg = cfg.normalized()
	if !cfg.IncludeStackTrace {
		return "", "", false
	}
	return "stack_trace", string(debug.Stack()), true
}
