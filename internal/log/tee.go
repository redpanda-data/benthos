// Copyright 2025 Redpanda Data, Inc.

package log

type teeLogger struct {
	a, b Modular
}

// TeeLogger creates a new log adapter that allows you to branch new modules.
func TeeLogger(a, b Modular) Modular {
	return &teeLogger{a: a, b: b}
}

// WithFields adds extra fields to the log adapter.
func (t *teeLogger) WithFields(fields map[string]string) Modular {
	return &teeLogger{
		a: t.a.WithFields(fields),
		b: t.b.WithFields(fields),
	}
}

// With returns a Logger that includes the given attributes. Arguments are
// converted to attributes as if by the standard `Logger.Log()`.
func (t *teeLogger) With(keyValues ...any) Modular {
	return &teeLogger{
		a: t.a.With(keyValues...),
		b: t.b.With(keyValues...),
	}
}

// Fatal logs at error level followed by a call to `os.Exit()`.
func (t *teeLogger) Fatal(format string, v ...any) {
	t.a.Fatal(format, v...)
	t.b.Fatal(format, v...)
}

// Error logs at error level.
func (t *teeLogger) Error(format string, v ...any) {
	t.a.Error(format, v...)
	t.b.Error(format, v...)
}

// Warn logs at warning level.
func (t *teeLogger) Warn(format string, v ...any) {
	t.a.Warn(format, v...)
	t.b.Warn(format, v...)
}

// Info logs at info level.
func (t *teeLogger) Info(format string, v ...any) {
	t.a.Info(format, v...)
	t.b.Info(format, v...)
}

// Debug logs at debug level.
func (t *teeLogger) Debug(format string, v ...any) {
	t.a.Debug(format, v...)
	t.b.Debug(format, v...)
}

// Trace logs at trace level.
func (t *teeLogger) Trace(format string, v ...any) {
	t.a.Trace(format, v...)
	t.b.Trace(format, v...)
}
