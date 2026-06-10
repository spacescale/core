package logger

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testStringer string

func (s testStringer) String() string {
	return string(s)
}

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
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, levelFor(tc.env))
		})
	}
}

func TestInitDevelopmentUsesTextHandlerAndShortensIDs(t *testing.T) {
	output := captureStdout(t, func() {
		log := Init("development").With("component", "test")
		log.Info("hello", "user_id", "1234567890abcdef")
	})

	assert.Contains(t, output, "level=INFO component=test msg=hello")
	assert.Contains(t, output, "user_id=12345678...")
	assert.NotContains(t, output, "1234567890abcdef")
}

func TestInitDevelopmentRecordComponentOverridesBaseComponent(t *testing.T) {
	output := captureStdout(t, func() {
		log := Init("development").With("component", "scaled")
		log.Info("nats connected", "component", "nats", "client", "scaled")
	})

	assert.Contains(t, output, "level=INFO component=nats msg=\"nats connected\" client=scaled")
	assert.NotContains(t, output, "component=scaled")
}

func TestInitProductionUsesJSONHandlerAndInfoLevel(t *testing.T) {
	output := captureStdout(t, func() {
		log := Init("production").With("component", "test")
		log.Debug("hidden", "user_id", "1234567890abcdef")
		log.Info("hello", "user_id", "1234567890abcdef")
	})

	assert.Contains(t, output, "\"level\":\"INFO\",\"component\":\"test\",\"msg\":\"hello\"")
	assert.Contains(t, output, "\"user_id\":\"1234567890abcdef\"")
	assert.NotContains(t, output, "hidden")
	assert.NotContains(t, output, "12345678...")
}

func TestOrderedHandlerEnabledHonorsConfiguredLevel(t *testing.T) {
	handler := newOrderedHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelWarn}, logFormatText)
	require.False(t, handler.Enabled(context.Background(), slog.LevelInfo))
	require.True(t, handler.Enabled(context.Background(), slog.LevelError))
}

func TestOrderedHandlerTextOutputIncludesGroupedAttrsAndComponent(t *testing.T) {
	buf := &bytes.Buffer{}
	handler := newOrderedHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}, logFormatText)
	groupedHandler, ok := handler.WithGroup("http").WithAttrs([]slog.Attr{slog.String("request_id", "req-123")}).(*orderedHandler)
	require.True(t, ok)
	handler = groupedHandler

	record := slog.NewRecord(time.Unix(0, 0), slog.LevelInfo, "hello world", 0)
	record.AddAttrs(
		slog.String("component", "api"),
		slog.Bool("ok", true),
	)

	require.NoError(t, handler.Handle(context.Background(), record))
	output := buf.String()
	assert.Contains(t, output, `level=INFO msg="hello world"`)
	assert.Contains(t, output, `http.request_id=req-123`)
	assert.Contains(t, output, `http.component=api`)
	assert.Contains(t, output, `http.ok=true`)
}

func TestOrderedHandlerJSONOutputIncludesSourceAndMarshalFallback(t *testing.T) {
	buf := &bytes.Buffer{}
	handler := newOrderedHandler(buf, &slog.HandlerOptions{AddSource: true, Level: slog.LevelDebug}, logFormatJSON)

	pc, _, _, ok := runtime.Caller(0)
	require.True(t, ok)

	record := slog.NewRecord(time.Unix(0, 0), slog.LevelInfo, "json hello", pc)
	record.AddAttrs(
		slog.String("component", "api"),
		slog.Any("bad", make(chan int)),
	)

	require.NoError(t, handler.Handle(context.Background(), record))
	output := buf.String()
	assert.Contains(t, output, `"component":"api"`)
	assert.Contains(t, output, `"msg":"json hello"`)
	assert.Contains(t, output, `"source":"`)
	assert.Contains(t, output, `"bad":"json: unsupported type: chan int"`)
}

func TestJSONValueAnySpecialCases(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		value := slog.AnyValue(errors.New("boom"))
		require.Equal(t, "boom", jsonValue(value))
	})

	t.Run("stringer", func(t *testing.T) {
		value := slog.AnyValue(testStringer("stringer-value"))
		require.Equal(t, "stringer-value", jsonValue(value))
	})
}

func TestAppendMaybeQuoted(t *testing.T) {
	assert.Equal(t, `plain`, string(appendMaybeQuoted(nil, "plain")))
	assert.Equal(t, `"two words"`, string(appendMaybeQuoted(nil, "two words")))
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
