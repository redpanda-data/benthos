// Copyright 2025 Redpanda Data, Inc.

package testutil

import (
	"fmt"

	"github.com/redpanda-data/benthos/v4/internal/log"
)

// MockLog is a mock log.Modular implementation.
type MockLog struct {
	Traces        []string
	Debugs        []string
	Infos         []string
	Warns         []string
	Errors        []string
	Fields        []map[string]string
	MappingFields []any
}

// WithFields adds fields to the MockLog message.
func (m *MockLog) WithFields(fields map[string]string) log.Modular {
	m.Fields = append(m.Fields, fields)
	return m
}

// With adds mapping fields to the MockLog message.
func (m *MockLog) With(args ...any) log.Modular {
	m.MappingFields = append(m.MappingFields, args...)
	return m
}

// Fatal logs a fatal error message with a given format.
func (m *MockLog) Fatal(format string, v ...any) {}

// Error logs an error message with a given format.
func (m *MockLog) Error(format string, v ...any) {
	m.Errors = append(m.Errors, fmt.Sprintf(format, v...))
}

// Warn logs a warning message with a given format.
func (m *MockLog) Warn(format string, v ...any) {
	m.Warns = append(m.Warns, fmt.Sprintf(format, v...))
}

// Info logs an info message with a given format.
func (m *MockLog) Info(format string, v ...any) {
	m.Infos = append(m.Infos, fmt.Sprintf(format, v...))
}

// Debug logs a debug message with a given format.
func (m *MockLog) Debug(format string, v ...any) {
	m.Debugs = append(m.Debugs, fmt.Sprintf(format, v...))
}

// Trace logs a trace message with a given format.
func (m *MockLog) Trace(format string, v ...any) {
	m.Traces = append(m.Traces, fmt.Sprintf(format, v...))
}

// Fatalln logs a fatal error message.
func (m *MockLog) Fatalln(message string) {}

// Errorln logs an error message.
func (m *MockLog) Errorln(message string) {
	m.Errors = append(m.Errors, message)
}

// Warnln logs a warning message.
func (m *MockLog) Warnln(message string) {
	m.Warns = append(m.Warns, message)
}

// Infoln logs an info message.
func (m *MockLog) Infoln(message string) {
	m.Infos = append(m.Infos, message)
}

// Debugln logs a debug message.
func (m *MockLog) Debugln(message string) {
	m.Debugs = append(m.Debugs, message)
}

// Traceln logs a trace message.
func (m *MockLog) Traceln(message string) {
	m.Traces = append(m.Traces, message)
}
