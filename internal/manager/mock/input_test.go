// Copyright 2025 Redpanda Data, Inc.

package mock_test

import (
	"github.com/redpanda-data/benthos/v4/internal/component/input"
	"github.com/redpanda-data/benthos/v4/internal/manager/mock"
)

var _ input.Streamed = &mock.Input{}
