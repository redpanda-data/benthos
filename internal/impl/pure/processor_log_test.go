// Copyright 2025 Redpanda Data, Inc.

package pure_test

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/internal/component/testutil"
	log_testutil "github.com/redpanda-data/benthos/v4/internal/log/testutil"
	"github.com/redpanda-data/benthos/v4/internal/manager/mock"
	"github.com/redpanda-data/benthos/v4/internal/message"
)

func TestLogBadLevel(t *testing.T) {
	conf, err := testutil.ProcessorFromYAML(`
log:
  level: does not exist
`)
	require.NoError(t, err)

	if _, err := mock.NewManager().NewProcessor(conf); err == nil {
		t.Error("expected err from bad log level")
	}
}

func TestLogLevelTrace(t *testing.T) {
	logMock := &log_testutil.MockLog{}

	levels := []string{"TRACE", "DEBUG", "INFO", "WARN", "ERROR"}
	for _, level := range levels {
		conf, err := testutil.ProcessorFromYAML(`
log:
  message: '${!json("foo")}'
  level: ` + level + `
`)
		require.NoError(t, err)

		mgr := mock.NewManager()
		mgr.L = logMock

		l, err := mgr.NewProcessor(conf)
		if err != nil {
			t.Fatal(err)
		}

		input := message.QuickBatch([][]byte{[]byte(fmt.Sprintf(`{"foo":"%v"}`, level))})
		expMsgs := []message.Batch{input}
		actMsgs, res := l.ProcessBatch(context.Background(), input)
		if res != nil {
			t.Fatal(res)
		}
		if !reflect.DeepEqual(expMsgs, actMsgs) {
			t.Errorf("Wrong message passthrough: %v != %v", actMsgs, expMsgs)
		}
	}

	if exp, act := []string{"TRACE"}, logMock.Traces; !reflect.DeepEqual(exp, act) {
		t.Errorf("Wrong log for trace: %v != %v", act, exp)
	}
	if exp, act := []string{"DEBUG"}, logMock.Debugs; !reflect.DeepEqual(exp, act) {
		t.Errorf("Wrong log for debug: %v != %v", act, exp)
	}
	if exp, act := []string{"INFO"}, logMock.Infos; !reflect.DeepEqual(exp, act) {
		t.Errorf("Wrong log for info: %v != %v", act, exp)
	}
	if exp, act := []string{"WARN"}, logMock.Warns; !reflect.DeepEqual(exp, act) {
		t.Errorf("Wrong log for warn: %v != %v", act, exp)
	}
	if exp, act := []string{"ERROR"}, logMock.Errors; !reflect.DeepEqual(exp, act) {
		t.Errorf("Wrong log for error: %v != %v", act, exp)
	}
}

func TestLogWithFields(t *testing.T) {
	conf, err := testutil.ProcessorFromYAML(`
log:
  message: '${!json("foo")}'
  level: INFO
  fields:
    static: foo
    dynamic: '${!json("bar")}'
`)
	require.NoError(t, err)

	logMock := &log_testutil.MockLog{}

	mgr := mock.NewManager()
	mgr.L = logMock

	l, err := mgr.NewProcessor(conf)
	if err != nil {
		t.Fatal(err)
	}

	input := message.QuickBatch([][]byte{[]byte(`{"foo":"info message","bar":"with fields"}`)})
	expMsgs := []message.Batch{input}
	actMsgs, res := l.ProcessBatch(context.Background(), input)
	if res != nil {
		t.Fatal(res)
	}
	if !reflect.DeepEqual(expMsgs, actMsgs) {
		t.Errorf("Wrong message passthrough: %v != %v", actMsgs, expMsgs)
	}

	if exp, act := []string{"info message"}, logMock.Infos; !reflect.DeepEqual(exp, act) {
		t.Errorf("Wrong log output: %v != %v", act, exp)
	}
	t.Logf("Checking %v\n", logMock.Fields)
	if exp, act := 1, len(logMock.Fields); exp != act {
		t.Fatalf("Wrong count of fields: %v != %v", act, exp)
	}
	if exp, act := map[string]string{"dynamic": "with fields", "static": "foo"}, logMock.Fields[0]; !reflect.DeepEqual(exp, act) {
		t.Errorf("Wrong field output: %v != %v", act, exp)
	}

	input = message.QuickBatch([][]byte{[]byte(`{"foo":"info message 2","bar":"with fields 2"}`)})
	expMsgs = []message.Batch{input}
	actMsgs, res = l.ProcessBatch(context.Background(), input)
	if res != nil {
		t.Fatal(res)
	}
	if !reflect.DeepEqual(expMsgs, actMsgs) {
		t.Errorf("Wrong message passthrough: %v != %v", actMsgs, expMsgs)
	}

	if exp, act := []string{"info message", "info message 2"}, logMock.Infos; !reflect.DeepEqual(exp, act) {
		t.Errorf("Wrong log output: %v != %v", act, exp)
	}
	t.Logf("Checking %v\n", logMock.Fields)
	if exp, act := 2, len(logMock.Fields); exp != act {
		t.Fatalf("Wrong count of fields: %v != %v", act, exp)
	}
	if exp, act := map[string]string{"dynamic": "with fields", "static": "foo"}, logMock.Fields[0]; !reflect.DeepEqual(exp, act) {
		t.Errorf("Wrong field output: %v != %v", act, exp)
	}
	if exp, act := map[string]string{"dynamic": "with fields 2", "static": "foo"}, logMock.Fields[1]; !reflect.DeepEqual(exp, act) {
		t.Errorf("Wrong field output: %v != %v", act, exp)
	}
}

func TestLogWithFieldsMapping(t *testing.T) {
	conf, err := testutil.ProcessorFromYAML(`
log:
  message: 'hello world'
  level: INFO
  fields_mapping: |
    root.static = "static value"
    root.age = this.age + 2
    root.is_cool = this.is_cool
`)
	require.NoError(t, err)

	logMock := &log_testutil.MockLog{}

	mgr := mock.NewManager()
	mgr.L = logMock

	l, err := mgr.NewProcessor(conf)
	require.NoError(t, err)

	input := message.QuickBatch([][]byte{[]byte(
		`{"age":10,"is_cool":true,"ignore":"this value please"}`,
	)})
	expMsgs := []message.Batch{input}
	actMsgs, res := l.ProcessBatch(context.Background(), input)
	require.NoError(t, res)
	assert.Equal(t, expMsgs, actMsgs)

	assert.Equal(t, []string{"hello world"}, logMock.Infos)
	assert.Equal(t, []any{
		"custom_source", true,
		"age", int64(12),
		"is_cool", true,
		"static", "static value",
	}, logMock.MappingFields)
}
