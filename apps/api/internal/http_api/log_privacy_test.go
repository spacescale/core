// This file tests user-agent log privacy helpers.
//
// Why these tests exist:
// - Log privacy behavior is shared across access, panic, and auth-failure logs.
// - A small regression here can leak raw user-agent values unexpectedly.
// - Explicit helper tests keep privacy mode behavior stable during refactors.
//
// Coverage focus:
// - mode-based field selection (hash, truncate, off)
// - empty/missing input handling
// - deterministic hash output for a stable secret and input
// - safe truncation behavior for raw user-agent output mode
// - panic-value truncation and stack-trace policy helpers

package http_api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/t0gun/spacescale/internal/config"
)

// TestDefaultLogPrivacyConfig verifies package defaults used when startup config
// is absent or invalid.
func TestDefaultLogPrivacyConfig(t *testing.T) {
	cfg := config.DefaultLogPrivacyConfig()

	require.Equal(t, config.UserAgentLogModeHash, cfg.UserAgentMode)
	require.Equal(t, 100, cfg.UserAgentMaxLen)
	require.Equal(t, 200, cfg.PanicValueMaxLen)
	require.False(t, cfg.IncludeStackTrace)
}

// TestUserAgentLogAttr verifies that user-agent privacy mode selection produces
// the expected structured log attribute shape.
func TestUserAgentLogAttr(t *testing.T) {
	const (
		testSecret = "privacy-test-secret"
		testUA     = "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_3_1)"
	)

	mac := hmac.New(sha256.New, []byte(testSecret))
	_, _ = mac.Write([]byte(testUA))
	wantHash := hex.EncodeToString(mac.Sum(nil))

	tests := []struct {
		name      string
		rawUA     string
		cfg       config.LogPrivacyConfig
		wantKey   string
		wantValue string
		wantOK    bool
	}{
		{
			name:  "off mode omits field",
			rawUA: testUA,
			cfg: config.LogPrivacyConfig{
				UserAgentMode: config.UserAgentLogModeOff,
			},
			wantOK: false,
		},
		{
			name:  "truncate mode keeps raw value within limit",
			rawUA: "yaak",
			cfg: config.LogPrivacyConfig{
				UserAgentMode:   config.UserAgentLogModeTruncate,
				UserAgentMaxLen: 100,
			},
			wantKey:   "user_agent",
			wantValue: "yaak",
			wantOK:    true,
		},
		{
			name:  "truncate mode shortens over limit value",
			rawUA: "postman-runtime",
			cfg: config.LogPrivacyConfig{
				UserAgentMode:   config.UserAgentLogModeTruncate,
				UserAgentMaxLen: 7,
			},
			wantKey:   "user_agent",
			wantValue: "postman",
			wantOK:    true,
		},
		{
			name:  "hash mode emits stable hmac field",
			rawUA: testUA,
			cfg: config.LogPrivacyConfig{
				UserAgentMode:       config.UserAgentLogModeHash,
				UserAgentHashSecret: testSecret,
			},
			wantKey:   "user_agent_hash",
			wantValue: wantHash,
			wantOK:    true,
		},
		{
			name:  "hash mode without secret omits field",
			rawUA: testUA,
			cfg: config.LogPrivacyConfig{
				UserAgentMode: config.UserAgentLogModeHash,
			},
			wantOK: false,
		},
		{
			name:  "empty user agent omits field",
			rawUA: "   ",
			cfg: config.LogPrivacyConfig{
				UserAgentMode: config.UserAgentLogModeTruncate,
			},
			wantOK: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			gotKey, gotValue, gotOK := userAgentLogAttr(tc.rawUA, tc.cfg)
			require.Equal(t, tc.wantOK, gotOK)
			if !tc.wantOK {
				require.Empty(t, gotKey)
				require.Empty(t, gotValue)
				return
			}

			require.Equal(t, tc.wantKey, gotKey)
			require.Equal(t, tc.wantValue, gotValue)
		})
	}
}

// TestTruncateLogString verifies rune-safe truncation behavior used by
// user-agent truncate mode.
func TestTruncateLogString(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{name: "no truncation needed", input: "abc", maxLen: 5, want: "abc"},
		{name: "ascii truncation", input: "abcdef", maxLen: 3, want: "abc"},
		{name: "zero max returns empty", input: "abcdef", maxLen: 0, want: ""},
		{name: "unicode truncation stays valid", input: "gopher🐹rocks", maxLen: 7, want: "gopher🐹"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := truncateLogString(tc.input, tc.maxLen)
			require.Equal(t, tc.want, got)
		})
	}
}

// TestPanicValueLogValue verifies panic value redaction behavior used by
// recoverer logging.
func TestPanicValueLogValue(t *testing.T) {
	tests := []struct {
		name      string
		recovered any
		cfg       config.LogPrivacyConfig
		want      string
	}{
		{
			name:      "uses default max length when unset",
			recovered: strings.Repeat("a", 250),
			cfg:       config.LogPrivacyConfig{},
			want:      strings.Repeat("a", 200),
		},
		{
			name:      "applies configured max length",
			recovered: "abcdefghijklmnopqrstuvwxyz",
			cfg: config.LogPrivacyConfig{
				PanicValueMaxLen: 5,
			},
			want: "abcde",
		},
		{
			name:      "non-string panic values are stringified",
			recovered: 12345,
			cfg: config.LogPrivacyConfig{
				PanicValueMaxLen: 10,
			},
			want: "12345",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := panicValueLogValue(tc.recovered, tc.cfg)
			require.Equal(t, tc.want, got)
		})
	}
}

// TestPanicStackTraceLogAttr verifies optional stack-trace behavior used by
// panic logging.
func TestPanicStackTraceLogAttr(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.LogPrivacyConfig
		wantKey string
		wantOK  bool
	}{
		{
			name:    "disabled by default",
			cfg:     config.LogPrivacyConfig{},
			wantKey: "",
			wantOK:  false,
		},
		{
			name: "enabled emits stack trace field",
			cfg: config.LogPrivacyConfig{
				IncludeStackTrace: true,
			},
			wantKey: "stack_trace",
			wantOK:  true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			gotKey, gotValue, gotOK := panicStackTraceLogAttr(tc.cfg)
			require.Equal(t, tc.wantOK, gotOK)
			if !tc.wantOK {
				require.Empty(t, gotKey)
				require.Empty(t, gotValue)
				return
			}

			require.Equal(t, tc.wantKey, gotKey)
			require.NotEmpty(t, gotValue)
		})
	}
}
