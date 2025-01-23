// Copyright 2025 Redpanda Data, Inc.

//go:build go1.21

package log

import (
	"fmt"
	"log/slog"
	"os"
)

type logHandler struct {
	slog *slog.Logger
}

// NewBenthosLogAdapter creates a new Benthos log adapter.
func NewBenthosLogAdapter(l *slog.Logger) *logHandler {
	return &logHandler{slog: l}
}

// WithFields adds extra fields to the log adapter.
func (l *logHandler) WithFields(fields map[string]string) Modular {
	tmp := l.slog
	for k, v := range fields {
		tmp = tmp.With(slog.String(k, v))
	}

	c := l.clone()
	c.slog = tmp
	return c
}

// With returns a Logger that includes the given attributes. Arguments are
// converted to attributes as if by the standard `Logger.Log()`.
func (l *logHandler) With(keyValues ...any) Modular {
	c := l.clone()
	c.slog = l.slog.With(keyValues...)
	return c
}

// Fatal logs at error level followed by a call to `os.Exit()`.
func (l *logHandler) Fatal(format string, v ...any) {
	l.slog.Error(fmt.Sprintf(format, v...))
	os.Exit(1)
}

// Error logs at error level.
func (l *logHandler) Error(format string, v ...any) {
	l.slog.Error(fmt.Sprintf(format, v...))
}

// Warn logs at warning level.
func (l *logHandler) Warn(format string, v ...any) {
	l.slog.Warn(fmt.Sprintf(format, v...))
}

// Info logs at info level.
func (l *logHandler) Info(format string, v ...any) {
	l.slog.Info(fmt.Sprintf(format, v...))
}

// Debug logs at debug level.
func (l *logHandler) Debug(format string, v ...any) {
	l.slog.Debug(fmt.Sprintf(format, v...))
}

// Trace logs at trace level.
func (l *logHandler) Trace(format string, v ...any) {
	l.slog.Debug(fmt.Sprintf(format, v...))
}

func (l *logHandler) clone() *logHandler {
	c := *l
	return &c
}
