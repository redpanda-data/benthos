// Copyright 2026 Redpanda Data, Inc.

package bloblangv2_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/redpanda-data/benthos/v4/public/bloblangv2"
)

// stubMessage is a minimal MessageContext used to drive the
// message-coupled stdlib in tests without standing up a full
// service.Message + pipeline.
type stubMessage struct {
	input      any
	meta       map[string]any
	bytes      []byte
	err        error
	batchIndex int
	batchSize  int
	traceID    string
	span       any
}

func (s *stubMessage) Input() any               { return s.input }
func (s *stubMessage) Metadata() map[string]any { return s.meta }
func (s *stubMessage) Bytes() []byte            { return s.bytes }
func (s *stubMessage) Error() error             { return s.err }
func (s *stubMessage) BatchIndex() int          { return s.batchIndex }
func (s *stubMessage) BatchSize() int           { return s.batchSize }
func (s *stubMessage) TraceID() string          { return s.traceID }
func (s *stubMessage) Span() any                { return s.span }

func TestQueryMessageBatchPosition(t *testing.T) {
	exec, err := bloblangv2.NewEnvironment().Parse(`
output.idx = batch_index()
output.size = batch_size()
`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got, _, err := exec.QueryMessage(&stubMessage{batchIndex: 2, batchSize: 5})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected object output, got %T", got)
	}
	if m["idx"] != int64(2) || m["size"] != int64(5) {
		t.Fatalf("unexpected output: %v", m)
	}
}

func TestQueryMessageContent(t *testing.T) {
	exec, err := bloblangv2.NewEnvironment().Parse(`output = content()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got, _, err := exec.QueryMessage(&stubMessage{bytes: []byte("hello")})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	b, ok := got.([]byte)
	if !ok {
		t.Fatalf("expected []byte output, got %T", got)
	}
	if string(b) != "hello" {
		t.Fatalf("got %q, want %q", string(b), "hello")
	}
}

func TestQueryMessageErrorObject(t *testing.T) {
	exec, err := bloblangv2.NewEnvironment().Parse(`
output.failed = errored()
output.err = error()
`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got, _, err := exec.QueryMessage(&stubMessage{err: errors.New("kapow")})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	m := got.(map[string]any)
	if m["failed"] != true {
		t.Fatalf("expected failed=true, got %v", m["failed"])
	}
	errObj, ok := m["err"].(map[string]any)
	if !ok {
		t.Fatalf("expected error to be an object, got %T", m["err"])
	}
	if errObj["what"] != "kapow" {
		t.Fatalf("expected what=kapow, got %v", errObj["what"])
	}
}

func TestQueryMessageNoErrorReturnsNull(t *testing.T) {
	exec, err := bloblangv2.NewEnvironment().Parse(`output.err = error()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got, _, err := exec.QueryMessage(&stubMessage{})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	m := got.(map[string]any)
	if m["err"] != nil {
		t.Fatalf("expected err=nil, got %v", m["err"])
	}
}

func TestQueryWithoutMessageErrorsOnMessageFunction(t *testing.T) {
	exec, err := bloblangv2.NewEnvironment().Parse(`output = batch_index()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	_, qerr := exec.Query(nil)
	if qerr == nil {
		t.Fatalf("expected error when calling message-coupled function via Query")
	}
	if !strings.Contains(qerr.Error(), "requires a message context") {
		t.Fatalf("error message did not mention message context: %v", qerr)
	}
}

func TestQueryMessageInputAndMetaStillBound(t *testing.T) {
	exec, err := bloblangv2.NewEnvironment().Parse(`
output.value = input.x
output.region = input@["region"]
`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got, _, err := exec.QueryMessage(&stubMessage{
		input: map[string]any{"x": int64(7)},
		meta:  map[string]any{"region": "eu-west"},
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	m := got.(map[string]any)
	if m["value"] != int64(7) {
		t.Fatalf("expected value=7, got %v", m["value"])
	}
	if m["region"] != "eu-west" {
		t.Fatalf("expected region=eu-west, got %v", m["region"])
	}
}
