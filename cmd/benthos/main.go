// Copyright 2025 Redpanda Data, Inc.

package main

import (
	"context"

	"github.com/redpanda-data/benthos/v4/public/service"

	// Import all plugins defined within the repo.
	_ "github.com/redpanda-data/benthos/v4/public/components/io"
	_ "github.com/redpanda-data/benthos/v4/public/components/pure"
	_ "github.com/redpanda-data/benthos/v4/public/components/pure/extended"
)

func main() {
	service.RunCLI(context.Background())
}
