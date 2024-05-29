package bloblang

import (
	"github.com/redpanda-data/benthos/v4/internal/bloblang/plugins"
)

func init() {
	if err := plugins.Register(); err != nil {
		panic(err)
	}
}
