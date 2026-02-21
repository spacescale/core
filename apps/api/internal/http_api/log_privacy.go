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

	"github.com/t0gun/spacescale/internal/config"
)

// userAgentLogAttr returns a log key/value for user-agent according to policy.
// It returns ok=false when user-agent logging is disabled or input is empty.
func userAgentLogAttr(rawUserAgent string, cfg config.LogPrivacyConfig) (string, string, bool) {
	cfg = cfg.Normalized()
	ua := strings.TrimSpace(rawUserAgent)
	if ua == "" {
		return "", "", false
	}

	switch cfg.UserAgentMode {
	case config.UserAgentLogModeOff:
		return "", "", false

	case config.UserAgentLogModeTruncate:
		return "user_agent", truncateLogString(ua, cfg.UserAgentMaxLen), true

	case config.UserAgentLogModeHash:
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
func panicValueLogValue(recovered any, cfg config.LogPrivacyConfig) string {
	cfg = cfg.Normalized()
	return truncateLogString(fmt.Sprint(recovered), cfg.PanicValueMaxLen)
}

// panicStackTraceLogAttr returns stack-trace log field when enabled by config.
func panicStackTraceLogAttr(cfg config.LogPrivacyConfig) (string, string, bool) {
	cfg = cfg.Normalized()
	if !cfg.IncludeStackTrace {
		return "", "", false
	}
	return "stack_trace", string(debug.Stack()), true
}
