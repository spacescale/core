// This file defines HTTP logging privacy configuration primitives.
//
// Why this file exists:
// - Keep log-privacy concepts cohesive instead of spreading mode/default logic
//   across router wiring and middleware implementation files.
// - Give startup code and middleware code one shared source of truth for
//   user-agent logging policy.
// - Give panic logging one shared privacy policy (panic value truncation and
//   optional stack traces) to reduce accidental sensitive-data exposure.
// - Make future log-privacy expansion easy to discover and maintain.
//
// Scope in this phase:
// - Defines user-agent log mode values.
// - Defines log-privacy runtime config struct and defaults.
// - Provides normalization rules so middleware wiring receives safe values.
// - Provides one helper that resolves the final structured log field according
//   to configured privacy mode.
// - Provides panic-specific helpers used by recoverer logging.

package http_api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"runtime/debug"
	"strings"
)

// UserAgentLogMode controls how user-agent data is represented in logs.
//
// Why this is a dedicated type:
// - Prevents ambiguous raw string usage in startup and middleware code.
// - Makes allowed values explicit and discoverable for maintainers.
// - Keeps future log-privacy extensions localized.
type UserAgentLogMode string

const (
	// UserAgentLogModeHash is intended to represent user-agent as a hashed value.
	// This mode is usually the safest production default for privacy.
	UserAgentLogModeHash UserAgentLogMode = "hash"
	// UserAgentLogModeTruncate is intended to keep only a shortened raw value.
	// This mode can be useful in local debugging scenarios.
	UserAgentLogModeTruncate UserAgentLogMode = "truncate"
	// UserAgentLogModeOff disables user-agent log output.
	// This mode is useful in strict-privacy environments.
	UserAgentLogModeOff UserAgentLogMode = "off"
)

const (
	// defaultUserAgentLogMode is used when startup config omits mode or provides
	// an unknown value.
	defaultUserAgentLogMode = UserAgentLogModeHash
	// defaultUserAgentMaxLength is used when truncate mode length is missing or
	// invalid.
	defaultUserAgentMaxLength = 100
	// defaultPanicValueMaxLength limits panic_value size in logs.
	// This helps avoid leaking large payload fragments and keeps log lines sane.
	defaultPanicValueMaxLength = 200
	// defaultIncludeStackTrace controls whether stack traces are included in panic
	// logs by default. Disabled by default for production safety.
	defaultIncludeStackTrace = false
)

// LogPrivacyConfig controls request-log privacy behavior for user-agent and
// panic-related fields.
//
// Field intent:
// - UserAgentMode selects one of the supported user-agent output strategies.
// - UserAgentHashSecret stores hashing key material for hash mode.
// - UserAgentMaxLen stores output length for truncate mode.
// - PanicValueMaxLen stores the maximum panic_value length to log.
// - IncludeStackTrace toggles stack trace output in panic logs.
//
// This struct is runtime-facing and transport-agnostic so startup can parse
// env vars once and pass typed config into server/router wiring.
type LogPrivacyConfig struct {
	UserAgentMode       UserAgentLogMode
	UserAgentHashSecret string
	UserAgentMaxLen     int
	PanicValueMaxLen    int
	IncludeStackTrace   bool
}

// DefaultLogPrivacyConfig returns package defaults for log-privacy behavior.
// Keeping defaults centralized avoids repeating magic values across startup,
// tests, and future binary entrypoints.
func DefaultLogPrivacyConfig() LogPrivacyConfig {
	return LogPrivacyConfig{
		UserAgentMode:     defaultUserAgentLogMode,
		UserAgentMaxLen:   defaultUserAgentMaxLength,
		PanicValueMaxLen:  defaultPanicValueMaxLength,
		IncludeStackTrace: defaultIncludeStackTrace,
	}
}

// normalized returns safe runtime config values for middleware wiring.
//
// Normalization rules:
// - Unknown UserAgentMode falls back to package default.
// - Non-positive UserAgentMaxLen falls back to package default.
// - Non-positive PanicValueMaxLen falls back to package default.
//
// This protects middleware behavior from accidental zero-value or invalid
// startup configuration.
func (c LogPrivacyConfig) normalized() LogPrivacyConfig {
	switch c.UserAgentMode {
	case UserAgentLogModeHash, UserAgentLogModeTruncate, UserAgentLogModeOff:
		// valid mode
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

// userAgentLogAttr resolves the final user-agent log attribute based on privacy
// configuration.
//
// Why this helper exists:
//   - Keeps user-agent privacy behavior centralized and consistent across access,
//     panic, and auth-failure logging paths.
//   - Avoids copy-pasting mode-specific branching in multiple middleware files.
//   - Makes future policy updates (new modes or additional redaction rules)
//     possible in one place.
//
// Return behavior:
// - ("user_agent_hash", <hmac>, true) in hash mode
// - ("user_agent", <truncated>, true) in truncate mode
// - ("", "", false) in off mode, empty input, or invalid hash prerequisites
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
			// Defensive fallback. Startup already validates this for hash mode.
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
//
// Rune-based truncation avoids cutting inside multi-byte UTF-8 code points,
// which keeps emitted log values valid and readable.
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

// panicValueLogValue returns a panic value string suitable for structured logs.
//
// Why this helper exists:
//   - Panic values can accidentally contain sensitive values or very large payload
//     fragments.
//   - Truncating to a bounded length keeps logs safer and operationally readable.
//
// Behavior:
// - Converts recovered panic value to string via fmt.Sprint.
// - Applies configured truncation length (with normalized defaults).
func panicValueLogValue(recovered any, cfg LogPrivacyConfig) string {
	cfg = cfg.normalized()
	return truncateLogString(fmt.Sprint(recovered), cfg.PanicValueMaxLen)
}

// panicStackTraceLogAttr returns the optional stack_trace log attribute.
//
// Why this helper exists:
//   - Production environments often avoid stack traces in logs to reduce exposure
//     of internal file paths and function names.
//   - Debug environments may still enable stack traces for faster incident triage.
//
// Return behavior:
// - ("stack_trace", <trace>, true) when stack traces are enabled.
// - ("", "", false) when disabled.
func panicStackTraceLogAttr(cfg LogPrivacyConfig) (string, string, bool) {
	cfg = cfg.normalized()
	if !cfg.IncludeStackTrace {
		return "", "", false
	}
	return "stack_trace", string(debug.Stack()), true
}
