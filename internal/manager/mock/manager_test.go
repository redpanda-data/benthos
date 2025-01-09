// Copyright 2025 Redpanda Data, Inc.

package mock_test

import (
	"github.com/redpanda-data/benthos/v4/internal/bundle"
	"github.com/redpanda-data/benthos/v4/internal/manager/mock"
)

var _ bundle.NewManagement = &mock.Manager{}
