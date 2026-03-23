package bloblang2

import (
	"testing"

	"github.com/redpanda-data/benthos/v4/internal/bloblspec"
)

func TestBloblangV2Spec(t *testing.T) {
	bloblspec.RunT(t, "../../resources/bloblang_v2/tests", &Interp{})
}
