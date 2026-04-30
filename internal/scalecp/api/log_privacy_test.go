// Copyright (c) 2026 SpaceScale Systems Inc. All rights reserved.

package api

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUserAgentLogAttr(t *testing.T) {
	tests := []struct {
		name      string
		rawUA     string
		wantKey   string
		wantValue string
		wantOK    bool
	}{
		{name: "plain text user agent is logged", rawUA: "yaak", wantKey: "user_agent", wantValue: "yaak", wantOK: true},
		{name: "long user agent is truncated", rawUA: strings.Repeat("a", 300), wantKey: "user_agent", wantValue: strings.Repeat("a", maxUserAgentLogLen), wantOK: true},
		{name: "empty user agent is omitted", rawUA: "   ", wantOK: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			gotKey, gotValue, gotOK := userAgentLogAttr(tc.rawUA)
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
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, truncateLogString(tc.input, tc.maxLen))
		})
	}
}

func TestPanicValueLogValue(t *testing.T) {
	require.Equal(t, strings.Repeat("a", maxPanicValueLogLen), panicValueLogValue(strings.Repeat("a", 300)))
	require.Equal(t, "12345", panicValueLogValue(12345))
}
