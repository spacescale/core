package logger

import (
	"log/slog"
	"os"
	"strings"
)

func Init(environment string) *slog.Logger {
	env := strings.ToLower(strings.TrimSpace(environment))
	opts := &slog.HandlerOptions{Level: levelFor(env)}

	var handler slog.Handler
	if env == "production" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		opts.ReplaceAttr = developmentReplaceAttr
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger
}

func developmentReplaceAttr(_ []string, attr slog.Attr) slog.Attr {
	if attr.Value.Kind() != slog.KindString {
		return attr
	}
	if !strings.HasSuffix(attr.Key, "_id") {
		return attr
	}
	attr.Value = slog.StringValue(shortID(attr.Value.String()))
	return attr
}

func shortID(v string) string {
	v = strings.TrimSpace(v)
	if len(v) <= 8 {
		return v
	}
	return v[:8] + "..."
}

func levelFor(environment string) slog.Level {
	if environment == "production" {
		return slog.LevelInfo
	}
	return slog.LevelDebug
}
