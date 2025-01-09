// Copyright 2025 Redpanda Data, Inc.

package mock_test

import (
	"github.com/redpanda-data/benthos/v4/internal/component/output"
	"github.com/redpanda-data/benthos/v4/internal/manager/mock"
)

var _ output.Sync = mock.OutputWriter(nil)
