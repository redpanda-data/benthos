// Copyright 2026 Redpanda Data, Inc.

package migrator_test

import (
	"context"
	"testing"

	"github.com/redpanda-data/benthos/v4/public/bloblangv2/migrator"
)

// TestMigrateNilContext guards that a nil context is tolerated (defaulted to
// Background) rather than panicking inside buildFileSet's unconditional
// ctx.Err() check.
func TestMigrateNilContext(t *testing.T) {
	var ctx context.Context // nil
	rep, err := migrator.Migrate(ctx, `root = this.foo`, migrator.Options{MinCoverage: 0})
	if err != nil {
		t.Fatalf("Migrate with nil ctx: unexpected error %v", err)
	}
	if rep == nil || rep.V2Mapping == "" {
		t.Fatal("Migrate with nil ctx produced no mapping")
	}
}
