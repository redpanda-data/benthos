// Copyright 2025 Redpanda Data, Inc.

package mock_test

import (
	"github.com/redpanda-data/benthos/v4/internal/component/processor"
	"github.com/redpanda-data/benthos/v4/internal/manager/mock"
)

var _ processor.V1 = mock.Processor(nil)
