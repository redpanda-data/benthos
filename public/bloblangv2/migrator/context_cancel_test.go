// Copyright 2026 Redpanda Data, Inc.

package migrator_test

import (
	"context"
	"errors"
	"testing"

	"github.com/redpanda-data/benthos/v4/public/bloblangv2/migrator"
)

// TestMigrateContextCancellation verifies the context threaded through Migrate
// is honoured during import-closure resolution: a cancelled context aborts the
// walk with the context error rather than proceeding with unbounded I/O.
func TestMigrateContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancelled

	resolverCalled := false
	_, err := migrator.Migrate(ctx, `import "helpers.blobl"`+"\n"+`root = foo()`, migrator.Options{
		MinCoverage: 0.0001,
		FileResolver: func(context.Context, string, string) (string, string, bool) {
			resolverCalled = true
			return "helpers.blobl", `map foo { root = "x" }`, true
		},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected the cancelled context to abort Migrate with context.Canceled, got %v", err)
	}
	if resolverCalled {
		t.Error("FileResolver should not have been consulted once the context was cancelled")
	}
}
