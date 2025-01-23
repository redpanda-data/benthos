// Copyright 2025 Redpanda Data, Inc.

package mock_test

import (
	"github.com/redpanda-data/benthos/v4/internal/component/ratelimit"
	"github.com/redpanda-data/benthos/v4/internal/manager/mock"
)

var _ ratelimit.V1 = mock.RateLimit(nil)
