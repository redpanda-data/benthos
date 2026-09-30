package eval_test

import (
	"testing"

	bloblangv2 "github.com/redpanda-data/benthos/v4/public/bloblangv2"
)

// TestUnboundedStdlibDoesNotCrash is the regression for the process-crash
// class: a mapping must never panic the host process. .repeat() and range()
// with pathological counts previously either panicked (int overflow, which
// Run's recover re-panicked) or allocated terabytes (OOM). They must now
// degrade to a message-level runtime error. The defer-recover in each run
// asserts that NO panic escapes the executor, regardless of the outcome.
func TestUnboundedStdlibDoesNotCrash(t *testing.T) {
	run := func(t *testing.T, src string) (out any, errStr string) {
		t.Helper()
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("panic escaped the executor (would crash the host process): %v", r)
			}
		}()
		ex, err := bloblangv2.GlobalEnvironment().Parse(src)
		if err != nil {
			t.Fatalf("compile: %v", err)
		}
		o, e := ex.Query(map[string]any{})
		if e != nil {
			return nil, e.Error()
		}
		return o, ""
	}

	t.Run("repeat overflow returns an error", func(t *testing.T) {
		if _, errStr := run(t, `output = "xx".repeat(9223372036854775807)`); errStr == "" {
			t.Fatal("expected a runtime error, got success")
		}
	})
	t.Run("repeat huge (would OOM) returns an error", func(t *testing.T) {
		if _, errStr := run(t, `output = "x".repeat(999999999999)`); errStr == "" {
			t.Fatal("expected a runtime error, got success")
		}
	})
	t.Run("range huge (would OOM) returns an error", func(t *testing.T) {
		if _, errStr := run(t, `output = range(0, 999999999999)`); errStr == "" {
			t.Fatal("expected a runtime error, got success")
		}
	})

	// Legitimate uses still work.
	t.Run("repeat within bounds works", func(t *testing.T) {
		out, errStr := run(t, `output = "ab".repeat(3)`)
		if errStr != "" || out != "ababab" {
			t.Fatalf("got out=%v err=%q, want ababab", out, errStr)
		}
	})
	t.Run("range within bounds works", func(t *testing.T) {
		out, errStr := run(t, `output = range(0, 4).length()`)
		if errStr != "" {
			t.Fatalf("unexpected error: %s", errStr)
		}
		if _, ok := out.(int64); !ok || out.(int64) != 4 {
			t.Fatalf("got %v (%T), want int64 4", out, out)
		}
	})
}
