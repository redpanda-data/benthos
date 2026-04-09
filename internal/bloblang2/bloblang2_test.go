package bloblang2

import (
	"testing"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/spectest"
)

func TestBloblangV2Spec(t *testing.T) {
	spectest.RunT(t, "spec/tests", &Interp{})
}

func TestBloblangV2Exam(t *testing.T) {
	spectest.RunT(t, "speccondenser/exam", &Interp{})
}
