package log

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/redpanda-data/benthos/v4/internal/bloblang/mapping"
	"github.com/redpanda-data/benthos/v4/internal/filepath/ifs"
	"github.com/redpanda-data/benthos/v4/internal/message"
)

var (
	_ Modular = (*Logger)(nil)
)

// Logger is an object with support for levelled logging and modular components.
type Logger struct {
	entry   *logrus.Entry
	mapping *mapping.Executor
}

// New returns a new logger functioning as a glorified wrapper around logrus.
// Sets up the logger from a config, or returns an error if the config
// is invalid.
func New(stream io.Writer, fs ifs.FS, config Config) (Modular, error) {
	if config.File.Path != "" {
		if config.File.Rotate {
			stream = &lumberjack.Logger{
				Filename:   config.File.Path,
				MaxSize:    10,
				MaxAge:     config.File.RotateMaxAge,
				MaxBackups: 1,
				Compress:   true,
			}
		} else {
			fw, err := ifs.OS().OpenFile(config.File.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o666)
			if err == nil {
				var isw bool
				if stream, isw = fw.(io.Writer); !isw {
					err = errors.New("failed to open a writeable file")
				}
			}
			if err != nil {
				return nil, err
			}
		}
	}

	logger := logrus.New()
	logger.Out = stream

	switch config.Format {
	case "json":
		logger.SetFormatter(&logrus.JSONFormatter{
			DisableTimestamp: !config.AddTimeStamp,
			FieldMap: logrus.FieldMap{
				logrus.FieldKeyTime:  config.TimestampName,
				logrus.FieldKeyMsg:   config.MessageName,
				logrus.FieldKeyLevel: config.LevelName,
			},
		})
	case "logfmt":
		logger.SetFormatter(&logrus.TextFormatter{
			DisableTimestamp: !config.AddTimeStamp,
			QuoteEmptyFields: true,
			FullTimestamp:    config.AddTimeStamp,
			FieldMap: logrus.FieldMap{
				logrus.FieldKeyTime:  config.TimestampName,
				logrus.FieldKeyMsg:   config.MessageName,
				logrus.FieldKeyLevel: config.LevelName,
			},
		})
	default:
		return nil, fmt.Errorf("log format '%v' not recognized", config.Format)
	}

	switch strings.ToUpper(config.LogLevel) {
	case "OFF", "NONE":
		logger.Level = logrus.PanicLevel
	case "FATAL":
		logger.Level = logrus.FatalLevel
	case "ERROR":
		logger.Level = logrus.ErrorLevel
	case "WARN":
		logger.Level = logrus.WarnLevel
	case "INFO":
		logger.Level = logrus.InfoLevel
	case "DEBUG":
		logger.Level = logrus.DebugLevel
	case "TRACE", "ALL":
		logger.Level = logrus.TraceLevel
		logger.Level = logrus.TraceLevel
	}

	sFields := logrus.Fields{}
	for k, v := range config.StaticFields {
		sFields[k] = v
	}
	logEntry := logger.WithFields(sFields)

	return &Logger{
		entry:   logEntry,
		mapping: config.Mapping,
	}, nil
}

//------------------------------------------------------------------------------

// Noop creates and returns a new logger object that writes nothing.
func Noop() Modular {
	logger := logrus.New()
	logger.Out = io.Discard
	return &Logger{entry: logger.WithFields(logrus.Fields{})}
}

// WithFields returns a logger with new fields added to the JSON formatted
// output.
func (l *Logger) WithFields(inboundFields map[string]string) Modular {
	newFields := make(logrus.Fields, len(inboundFields))
	for k, v := range inboundFields {
		newFields[k] = v
	}

	newLogger := *l
	newLogger.entry = l.entry.WithFields(newFields)
	return &newLogger
}

// With returns a copy of the logger with new labels added to the logging
// context.
func (l *Logger) With(keyValues ...any) Modular {
	newEntry := l.entry.WithFields(logrus.Fields{})
	for i := 0; i < (len(keyValues) - 1); i += 2 {
		key, ok := keyValues[i].(string)
		if !ok {
			continue
		}
		newEntry = newEntry.WithField(key, keyValues[i+1])
	}

	newLogger := *l
	newLogger.entry = newEntry
	return &newLogger
}

//------------------------------------------------------------------------------

// Fatal prints a fatal message to the console. Does NOT cause panic.
func (l *Logger) Fatal(format string, v ...any) {

	format = strings.TrimSuffix(format, "\n")

	if l.mapping == nil {
		l.entry.Fatalf(format, v...)
		return
	}

	previousEntry := l.saveEntry()
	defer l.restoreEntry(previousEntry)

	payload := l.mustMapInBloblang(map[string]any{
		"fatal": map[string]any{
			"fields": map[string]any(l.entry.Data),
			"format": format,
		},
	})

	l.logByType(payload, v...)
}

// Error prints an error message to the console.
func (l *Logger) Error(format string, v ...any) {

	format = strings.TrimSuffix(format, "\n")

	if l.mapping == nil {
		l.entry.Errorf(format, v...)
		return
	}

	previousEntry := l.saveEntry()
	defer l.restoreEntry(previousEntry)

	payload := l.mustMapInBloblang(map[string]any{
		"error": map[string]any{
			"fields": map[string]any(l.entry.Data),
			"format": format,
		},
	})

	l.logByType(payload, v...)
}

// Warn prints a warning message to the console.
func (l *Logger) Warn(format string, v ...any) {

	format = strings.TrimSuffix(format, "\n")

	if l.mapping == nil {
		l.entry.Warnf(format, v...)
		return
	}

	previousEntry := l.saveEntry()
	defer l.restoreEntry(previousEntry)

	payload := l.mustMapInBloblang(map[string]any{
		"warn": map[string]any{
			"fields": map[string]any(l.entry.Data),
			"format": format,
		},
	})

	l.logByType(payload, v...)
}

// Info prints an information message to the console.
func (l *Logger) Info(format string, v ...any) {

	format = strings.TrimSuffix(format, "\n")

	if l.mapping == nil {
		l.entry.Infof(format, v...)
		return
	}

	previousEntry := l.saveEntry()
	defer l.restoreEntry(previousEntry)

	payload := l.mustMapInBloblang(map[string]any{
		"info": map[string]any{
			"fields": map[string]any(l.entry.Data),
			"format": format,
		},
	})

	l.logByType(payload, v...)
}

// Debug prints a debug message to the console.
func (l *Logger) Debug(format string, v ...any) {

	format = strings.TrimSuffix(format, "\n")

	if l.mapping == nil {
		l.entry.Debugf(format, v...)
		return
	}

	previousEntry := l.saveEntry()
	defer l.restoreEntry(previousEntry)

	payload := l.mustMapInBloblang(map[string]any{
		"debug": map[string]any{
			"fields": map[string]any(l.entry.Data),
			"format": format,
		},
	})

	l.logByType(payload, v...)
}

// Trace prints a trace message to the console.
func (l *Logger) Trace(format string, v ...any) {

	format = strings.TrimSuffix(format, "\n")

	if l.mapping == nil {
		l.entry.Tracef(format, v...)
		return
	}

	previousEntry := l.saveEntry()
	defer l.restoreEntry(previousEntry)

	payload := l.mustMapInBloblang(map[string]any{
		"trace": map[string]any{
			"fields": map[string]any(l.entry.Data),
			"format": format,
		},
	})

	l.logByType(payload, v...)
}

func (l *Logger) logByType(m map[string]any, v ...any) {
	if m == nil {
		return
	}

	inputValid := false

	if debug, ok := m["debug"]; ok {
		inputValid = true
		m := debug.(map[string]any)
		func() {
			entry := l.saveEntry()
			l.setFieldsFromMapped(m)
			defer l.restoreEntry(entry)
			l.entry.Debugf(m["format"].(string), v...)
		}()
	}

	if ev, ok := m["error"]; ok {
		inputValid = true
		m := ev.(map[string]any)
		func() {
			entry := l.saveEntry()
			l.setFieldsFromMapped(m)
			defer l.restoreEntry(entry)
			l.entry.Errorf(m["format"].(string), v...)
		}()
	}

	if fatal, ok := m["fatal"]; ok {
		inputValid = true
		m := fatal.(map[string]any)
		func() {
			entry := l.saveEntry()
			l.setFieldsFromMapped(m)
			defer l.restoreEntry(entry)
			l.entry.Fatalf(m["format"].(string), v...)
		}()
	}

	if info, ok := m["info"]; ok {
		inputValid = true
		m := info.(map[string]any)
		func() {
			entry := l.saveEntry()
			l.setFieldsFromMapped(m)
			defer l.restoreEntry(entry)
			l.entry.Infof(m["format"].(string), v...)
		}()
	}

	if warn, ok := m["warn"]; ok {
		inputValid = true
		m := warn.(map[string]any)
		func() {
			entry := l.saveEntry()
			l.setFieldsFromMapped(m)
			defer l.restoreEntry(entry)
			l.entry.Warnf(m["format"].(string), v...)
		}()
	}

	if trace, ok := m["trace"]; ok {
		inputValid = true
		m := trace.(map[string]any)
		func() {
			entry := l.saveEntry()
			l.setFieldsFromMapped(m)
			defer l.restoreEntry(entry)
			l.entry.Tracef(m["format"].(string), v...)
		}()
	}

	if !inputValid {
		panic("log mapping invalid. Needs to log one severity or delete response")
	}
}

func (l *Logger) mustMapInBloblang(payload map[string]any) map[string]any {
	payload, err := l.mapInBloblang(payload)
	if err != nil {
		panic(err)
	}
	return payload
}

func (l *Logger) mapInBloblang(payload map[string]any) (map[string]any, error) {

	l.entry = l.entry.WithFields(logrus.Fields{})

	part := message.NewPart(nil)
	part.SetStructuredMut(payload)

	mappedPart, err := bloblangQuery(l.mapping, part)
	if err != nil {
		return nil, fmt.Errorf("bloblangQuery failed: %w", err)
	}

	if mappedPart == nil {
		return nil, nil
	}

	result, err := mappedPart.AsStructured()
	if err != nil {
		return nil, fmt.Errorf("extracting structured result failed: %w", err)
	}

	return result.(map[string]any), nil
}

func (l *Logger) saveEntry() logrus.Entry {
	return *l.entry
}

func (l *Logger) restoreEntry(entry logrus.Entry) {
	l.entry = &entry
}

func (l *Logger) setFieldsFromMapped(mapped map[string]any) {
	fields := mapped["fields"].(map[string]any)
	l.entry = l.entry.WithFields(fields)
}

// bloblangQuery executes a parsed Bloblang mapping on a message and returns a
// message back or an error if the mapping fails. If the mapping results in the
// root being deleted the returned message will be nil, which indicates it has
// been filtered.
func bloblangQuery(blobl *mapping.Executor, part *message.Part) (*message.Part, error) {

	msg := message.Batch{part}

	return blobl.MapPart(0, msg)
}
