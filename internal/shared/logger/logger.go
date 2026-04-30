// Copyright (c) 2026 SpaceScale Systems Inc. All rights reserved.

package logger

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

type logFormat uint8

const (
	logFormatText logFormat = iota
	logFormatJSON
)

const componentKey = "component"

func Init(environment string) *slog.Logger {
	env := strings.ToLower(strings.TrimSpace(environment))
	opts := &slog.HandlerOptions{Level: levelFor(env)}

	var format logFormat
	if env == "production" {
		format = logFormatJSON
	} else {
		opts.ReplaceAttr = developmentReplaceAttr
		format = logFormatText
	}

	handler := newOrderedHandler(os.Stdout, opts, format)
	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger
}

type orderedHandler struct {
	out    io.Writer
	opts   slog.HandlerOptions
	format logFormat
	mu     *sync.Mutex
	attrs  []slog.Attr
	groups []string
}

func newOrderedHandler(out io.Writer, opts *slog.HandlerOptions, format logFormat) *orderedHandler {
	var copied slog.HandlerOptions
	if opts != nil {
		copied = *opts
	}
	return &orderedHandler{
		out:    out,
		opts:   copied,
		format: format,
		mu:     &sync.Mutex{},
	}
}

func (h *orderedHandler) Enabled(_ context.Context, level slog.Level) bool {
	min := slog.LevelInfo
	if h.opts.Level != nil {
		min = h.opts.Level.Level()
	}
	return level >= min
}

func (h *orderedHandler) Handle(_ context.Context, record slog.Record) error {
	attrs, component := h.collectAttrs(record)
	buf := make([]byte, 0, 512)

	switch h.format {
	case logFormatJSON:
		buf = appendJSONRecord(buf, record, component, attrs, h.opts.AddSource)
	default:
		buf = appendTextRecord(buf, record, component, attrs)
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.out.Write(buf)
	return err
}

func (h *orderedHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := *h
	clone.attrs = slices.Clone(h.attrs)
	for _, attr := range attrs {
		clone.attrs = append(clone.attrs, h.prepareAttr(attr))
	}
	return &clone
}

func (h *orderedHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	clone := *h
	clone.groups = append(slices.Clone(h.groups), name)
	return &clone
}

func (h *orderedHandler) collectAttrs(record slog.Record) ([]slog.Attr, string) {
	attrs := slices.Clone(h.attrs)
	record.Attrs(func(attr slog.Attr) bool {
		attrs = append(attrs, h.prepareAttr(attr))
		return true
	})

	component := ""
	filtered := attrs[:0]
	for _, attr := range attrs {
		if attr.Key == componentKey {
			component = attr.Value.String()
			continue
		}
		filtered = append(filtered, attr)
	}
	return filtered, component
}

func (h *orderedHandler) prepareAttr(attr slog.Attr) slog.Attr {
	attr.Value = attr.Value.Resolve()
	if len(h.groups) != 0 {
		attr.Key = strings.Join(append(slices.Clone(h.groups), attr.Key), ".")
	}
	if h.opts.ReplaceAttr != nil {
		attr = h.opts.ReplaceAttr(nil, attr)
	}
	return attr
}

func appendTextRecord(buf []byte, record slog.Record, component string, attrs []slog.Attr) []byte {
	if !record.Time.IsZero() {
		buf = appendTextAttr(buf, slog.String(slog.TimeKey, record.Time.Format(time.RFC3339Nano)))
	}
	buf = appendTextAttr(buf, slog.String(slog.LevelKey, record.Level.String()))
	if component != "" {
		buf = appendTextAttr(buf, slog.String(componentKey, component))
	}
	buf = appendTextAttr(buf, slog.String(slog.MessageKey, record.Message))
	for _, attr := range attrs {
		buf = appendTextAttr(buf, attr)
	}
	return append(buf, '\n')
}

func appendTextAttr(buf []byte, attr slog.Attr) []byte {
	if len(buf) != 0 {
		buf = append(buf, ' ')
	}
	buf = append(buf, attr.Key...)
	buf = append(buf, '=')
	return appendTextValue(buf, attr.Value)
}

func appendTextValue(buf []byte, value slog.Value) []byte {
	switch value.Kind() {
	case slog.KindString:
		return appendMaybeQuoted(buf, value.String())
	case slog.KindBool:
		return strconv.AppendBool(buf, value.Bool())
	case slog.KindInt64:
		return strconv.AppendInt(buf, value.Int64(), 10)
	case slog.KindUint64:
		return strconv.AppendUint(buf, value.Uint64(), 10)
	case slog.KindFloat64:
		return strconv.AppendFloat(buf, value.Float64(), 'g', -1, 64)
	case slog.KindDuration:
		return append(buf, value.Duration().String()...)
	case slog.KindTime:
		return append(buf, value.Time().Format(time.RFC3339Nano)...)
	case slog.KindAny:
		return appendMaybeQuoted(buf, fmt.Sprint(value.Any()))
	default:
		return appendMaybeQuoted(buf, value.String())
	}
}

func appendMaybeQuoted(buf []byte, value string) []byte {
	if value == "" || strings.ContainsAny(value, " \t\n\r=\"") {
		return strconv.AppendQuote(buf, value)
	}
	return append(buf, value...)
}

func appendJSONRecord(buf []byte, record slog.Record, component string, attrs []slog.Attr, addSource bool) []byte {
	buf = append(buf, '{')
	first := true
	if !record.Time.IsZero() {
		buf, first = appendJSONAttr(buf, first, slog.String(slog.TimeKey, record.Time.Format(time.RFC3339Nano)))
	}
	buf, first = appendJSONAttr(buf, first, slog.String(slog.LevelKey, record.Level.String()))
	if component != "" {
		buf, first = appendJSONAttr(buf, first, slog.String(componentKey, component))
	}
	buf, first = appendJSONAttr(buf, first, slog.String(slog.MessageKey, record.Message))
	for _, attr := range attrs {
		buf, first = appendJSONAttr(buf, first, attr)
	}
	if addSource && record.PC != 0 {
		buf, first = appendJSONAttr(buf, first, slog.String(slog.SourceKey, sourceLocation(record.PC)))
	}
	_ = first
	buf = append(buf, '}', '\n')
	return buf
}

func appendJSONAttr(buf []byte, first bool, attr slog.Attr) ([]byte, bool) {
	if !first {
		buf = append(buf, ',')
	}
	key, _ := json.Marshal(attr.Key)
	value, _ := json.Marshal(jsonValue(attr.Value))
	buf = append(buf, key...)
	buf = append(buf, ':')
	buf = append(buf, value...)
	return buf, false
}

func jsonValue(value slog.Value) any {
	switch value.Kind() {
	case slog.KindString:
		return value.String()
	case slog.KindBool:
		return value.Bool()
	case slog.KindInt64:
		return value.Int64()
	case slog.KindUint64:
		return value.Uint64()
	case slog.KindFloat64:
		return value.Float64()
	case slog.KindDuration:
		return value.Duration().String()
	case slog.KindTime:
		return value.Time().Format(time.RFC3339Nano)
	case slog.KindAny:
		switch v := value.Any().(type) {
		case error:
			return v.Error()
		case fmt.Stringer:
			return v.String()
		}
		return value.Any()
	default:
		return value.String()
	}
}

func sourceLocation(pc uintptr) string {
	frame, _ := runtime.CallersFrames([]uintptr{pc}).Next()
	if frame.File == "" {
		return ""
	}
	return frame.File + ":" + strconv.Itoa(frame.Line)
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
