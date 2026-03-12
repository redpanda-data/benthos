// Copyright 2025 Redpanda Data, Inc.

package service_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/public/service"
)

// dbPool is a fake database pool resource used to demonstrate custom resource
// registration and access from within a processor.
type dbPool struct {
	DSN string

	mu     sync.Mutex
	closed bool
}

func (p *dbPool) Close(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	return nil
}

func (p *dbPool) isClosed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closed
}

func TestCustomResourceEndToEnd(t *testing.T) {
	// Track constructed pools so we can inspect them after the stream runs.
	var (
		mu    sync.Mutex
		pools []*dbPool
	)

	env := service.NewEnvironment()

	// 1. Register a custom resource type "db_pools" with a "dsn" field.
	require.NoError(t, env.RegisterResource(
		"db_pools",
		[]*service.ConfigField{
			service.NewStringField("dsn").Description("Database connection string."),
		},
		func(conf *service.ParsedConfig, mgr *service.Resources) (any, error) {
			dsn, err := conf.FieldString("dsn")
			if err != nil {
				return nil, err
			}
			pool := &dbPool{DSN: dsn}
			mu.Lock()
			pools = append(pools, pool)
			mu.Unlock()
			return pool, nil
		},
	))

	// 2. Register a processor that looks up a db_pool resource and stamps the
	//    DSN onto every message it processes.
	require.NoError(t, env.RegisterProcessor(
		"stamp_dsn",
		service.NewConfigSpec().
			Field(service.NewStringField("pool_label").Description("Label of the db_pools resource to use.")),
		func(conf *service.ParsedConfig, mgr *service.Resources) (service.Processor, error) {
			label, err := conf.FieldString("pool_label")
			if err != nil {
				return nil, err
			}
			raw, ok := mgr.AccessResource("db_pools", label)
			if !ok {
				return nil, fmt.Errorf("db_pools resource %q not found", label)
			}
			pool := raw.(*dbPool)
			return &dsnStamper{pool: pool}, nil
		},
	))

	// 3. Build and run a stream using YAML that references the custom resource.
	b := env.NewStreamBuilder()
	b.SetSchema(env.FullConfigSchema("", ""))

	require.NoError(t, b.SetYAML(`
db_pools:
  - label: primary
    dsn: "postgres://localhost:5432/mydb"
  - label: analytics
    dsn: "postgres://localhost:5432/analytics"

input:
  generate:
    count: 1
    interval: 1ms
    mapping: 'root.msg = "hello"'
  processors:
    - stamp_dsn:
        pool_label: primary

output:
  drop: {}

logger:
  level: none
`))

	var captured []string
	require.NoError(t, b.AddConsumerFunc(func(ctx context.Context, m *service.Message) error {
		b, err := m.AsBytes()
		if err != nil {
			return err
		}
		captured = append(captured, string(b))
		return nil
	}))

	strm, err := b.Build()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, strm.Run(ctx))

	// 4. Verify the processor accessed the resource and stamped the DSN.
	require.Len(t, captured, 1)
	assert.Contains(t, captured[0], "postgres://localhost:5432/mydb")

	// 5. Verify both pools were constructed and closed on shutdown.
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, pools, 2)
	for _, p := range pools {
		assert.True(t, p.isClosed(), "pool %s should be closed after shutdown", p.DSN)
	}
}

// dsnStamper is a processor that replaces message content with the pool's DSN.
type dsnStamper struct {
	pool *dbPool
}

func (d *dsnStamper) Process(_ context.Context, m *service.Message) (service.MessageBatch, error) {
	m.SetBytes([]byte(d.pool.DSN))
	return service.MessageBatch{m}, nil
}

func (d *dsnStamper) Close(context.Context) error {
	return nil
}
