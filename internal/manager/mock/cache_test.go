// Copyright 2025 Redpanda Data, Inc.

package mock_test

import (
	"github.com/redpanda-data/benthos/v4/internal/component/cache"
	"github.com/redpanda-data/benthos/v4/internal/manager/mock"
)

var _ cache.V1 = &mock.Cache{}
