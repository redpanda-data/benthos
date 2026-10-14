package v1spec_test

import (
	"testing"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/v1spec"
)

// TestBloblangV1Spec runs every YAML test under ./tests against the V1
// Bloblang interpreter, using the shared spectest schema. Tests marked with a
// `skip:` field in the YAML are reported via t.Skip and do not execute.
func TestBloblangV1Spec(t *testing.T) {
	v1spec.RunT(t, "tests", v1spec.V1Interp{})
}
