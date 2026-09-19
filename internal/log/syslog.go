// Copyright 2025 Redpanda Data, Inc.

package log

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type syslogModular struct {
	conn     net.Conn
	mu       *sync.Mutex
	facility int
	tag      string
	hostname string
	pid      int
	fields   map[string]string
}

func newSyslogModular(cfg Config) (*syslogModular, error) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "localhost"
	}

	facility, err := syslogFacilityCode(cfg.Syslog.Facility)
	if err != nil {
		return nil, err
	}

	conn, err := net.Dial(cfg.Syslog.Transport, net.JoinHostPort(cfg.Syslog.Host, strconv.Itoa(cfg.Syslog.Port)))
	if err != nil {
		return nil, fmt.Errorf("dialing syslog server: %w", err)
	}

	return &syslogModular{
		conn:     conn,
		mu:       &sync.Mutex{},
		facility: facility,
		tag:      cfg.Syslog.Tag,
		hostname: hostname,
		pid:      os.Getpid(),
		fields:   map[string]string{},
	}, nil
}

func syslogFacilityCode(facility string) (int, error) {
	switch strings.ToLower(facility) {
	case "user":
		return 1, nil
	case "local0":
		return 16, nil
	case "local1":
		return 17, nil
	case "local2":
		return 18, nil
	case "local3":
		return 19, nil
	case "local4":
		return 20, nil
	case "local5":
		return 21, nil
	case "local6":
		return 22, nil
	case "local7":
		return 23, nil
	default:
		return 0, fmt.Errorf("unknown syslog facility %q", facility)
	}
}

func syslogSeverity(methodName string) int {
	switch methodName {
	case "fatal", "error":
		return 3
	case "warn":
		return 4
	case "info":
		return 6
	default: // debug, trace
		return 7
	}
}

func (s *syslogModular) write(severity int, msg string) {
	priority := s.facility*8 + severity
	timestamp := time.Now().UTC().Format(time.RFC3339Nano)

	var sb strings.Builder
	sb.WriteString(strings.TrimSuffix(msg, "\n"))
	for k, v := range s.fields {
		sb.WriteByte(' ')
		sb.WriteString(k)
		sb.WriteByte('=')
		sb.WriteString(v)
	}

	line := fmt.Sprintf("<%d>1 %s %s %s %d - - %s\n",
		priority,
		timestamp,
		s.hostname,
		s.tag,
		s.pid,
		sb.String(),
	)

	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = fmt.Fprint(s.conn, line)
}

// WithFields returns a copy of the logger with the given fields merged in.
func (s *syslogModular) WithFields(inboundFields map[string]string) Modular {
	merged := make(map[string]string, len(s.fields)+len(inboundFields))
	for k, v := range s.fields {
		merged[k] = v
	}
	for k, v := range inboundFields {
		merged[k] = v
	}
	cp := *s
	cp.fields = merged
	return &cp
}

// With returns a copy of the logger with the given key/value pairs merged in.
func (s *syslogModular) With(keyValues ...any) Modular {
	merged := make(map[string]string, len(s.fields))
	for k, v := range s.fields {
		merged[k] = v
	}
	for i := 0; i < len(keyValues)-1; i += 2 {
		key, ok := keyValues[i].(string)
		if !ok {
			continue
		}
		merged[key] = fmt.Sprintf("%v", keyValues[i+1])
	}
	cp := *s
	cp.fields = merged
	return &cp
}

func (s *syslogModular) log(severity int, format string, v ...any) {
	msg := format
	if len(v) > 0 {
		msg = fmt.Sprintf(format, v...)
	}
	s.write(severity, msg)
}

// Fatal logs at fatal severity.
func (s *syslogModular) Fatal(format string, v ...any) {
	s.log(syslogSeverity("fatal"), format, v...)
}

// Error logs at error severity.
func (s *syslogModular) Error(format string, v ...any) {
	s.log(syslogSeverity("error"), format, v...)
}

// Warn logs at warning severity.
func (s *syslogModular) Warn(format string, v ...any) {
	s.log(syslogSeverity("warn"), format, v...)
}

// Info logs at informational severity.
func (s *syslogModular) Info(format string, v ...any) {
	s.log(syslogSeverity("info"), format, v...)
}

// Debug logs at debug severity.
func (s *syslogModular) Debug(format string, v ...any) {
	s.log(syslogSeverity("debug"), format, v...)
}

// Trace logs at trace severity.
func (s *syslogModular) Trace(format string, v ...any) {
	s.log(syslogSeverity("trace"), format, v...)
}

// Close closes the underlying network connection.
func (s *syslogModular) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conn.Close()
}
