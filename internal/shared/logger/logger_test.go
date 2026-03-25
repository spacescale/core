package logger

import (
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShortID(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "short id unchanged", in: "abc123", want: "abc123"},
		{name: "trimmed long id shortened", in: "  1234567890abcdef  ", want: "12345678..."},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, shortID(tc.in))
		})
	}
}

func TestDevelopmentReplaceAttr(t *testing.T) {
	tests := []struct {
		name string
		attr slog.Attr
		want string
	}{
		{name: "shortens id suffix", attr: slog.String("user_id", "1234567890abcdef"), want: "12345678..."},
		{name: "leaves non id key", attr: slog.String("user", "1234567890abcdef"), want: "1234567890abcdef"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := developmentReplaceAttr(nil, tc.attr)
			assert.Equal(t, tc.attr.Key, got.Key)
			assert.Equal(t, tc.want, got.Value.String())
		})
	}
}

func TestLevelFor(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want slog.Level
	}{
		{name: "production", env: "production", want: slog.LevelInfo},
		{name: "non production", env: "development", want: slog.LevelDebug},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, levelFor(tc.env))
		})
	}
}

func TestInitDevelopmentUsesTextHandlerAndShortensIDs(t *testing.T) {
	output := captureStdout(t, func() {
		log := Init("development")
		log.Info("hello", "user_id", "1234567890abcdef")
	})

	assert.Contains(t, output, "level=INFO")
	assert.Contains(t, output, "msg=hello")
	assert.Contains(t, output, "user_id=12345678...")
	assert.NotContains(t, output, "1234567890abcdef")
}

func TestInitProductionUsesJSONHandlerAndInfoLevel(t *testing.T) {
	output := captureStdout(t, func() {
		log := Init("production")
		log.Debug("hidden", "user_id", "1234567890abcdef")
		log.Info("hello", "user_id", "1234567890abcdef")
	})

	assert.Contains(t, output, "\"level\":\"INFO\"")
	assert.Contains(t, output, "\"msg\":\"hello\"")
	assert.Contains(t, output, "\"user_id\":\"1234567890abcdef\"")
	assert.NotContains(t, output, "hidden")
	assert.NotContains(t, output, "12345678...")
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	oldStdout := os.Stdout
	oldLogger := slog.Default()
	r, w, err := os.Pipe()
	require.NoError(t, err)

	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = oldStdout
		slog.SetDefault(oldLogger)
	})

	fn()

	require.NoError(t, w.Close())
	output, err := io.ReadAll(r)
	require.NoError(t, err)
	require.NoError(t, r.Close())

	return string(output)
}
