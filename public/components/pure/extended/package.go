// Copyright 2025 Redpanda Data, Inc.

// Package extended contains component implementations that have a larger
// dependency footprint but do not interact with external systems (so an
// extension of pure components)
//
// EXPERIMENTAL: The specific components excluded by this package may change
// outside of major version releases. This means we may choose to remove certain
// plugins if we determine that their dependencies are likely to interfere with
// the goals of this package.
package extended

import (
	// Import pure but larger packages.
	_ "github.com/redpanda-data/benthos/v4/internal/impl/pure/extended"
)
