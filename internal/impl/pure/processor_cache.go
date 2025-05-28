// Copyright 2025 Redpanda Data, Inc.

package pure

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redpanda-data/benthos/v4/internal/bloblang/field"
	"github.com/redpanda-data/benthos/v4/internal/bundle"
	"github.com/redpanda-data/benthos/v4/internal/component"
	"github.com/redpanda-data/benthos/v4/internal/component/cache"
	"github.com/redpanda-data/benthos/v4/internal/component/interop"
	"github.com/redpanda-data/benthos/v4/internal/component/processor"
	"github.com/redpanda-data/benthos/v4/internal/message"
	"github.com/redpanda-data/benthos/v4/public/service"
)

const (
	cachePFieldResource = "resource"
	cachePFieldOperator = "operator"
	cachePFieldKey      = "key"
	cachePFieldValue    = "value"
	cachePFieldTTL      = "ttl"
)

func cacheProcSpec() *service.ConfigSpec {
	return service.NewConfigSpec().
		Categories("Integration").
		Stable().
		Summary("Performs operations against a xref:components:caches/about.adoc[cache resource] for each message, allowing you to store or retrieve data within message payloads.").
		Description(`
For use cases where you wish to cache the result of processors consider using the `+"xref:components:processors/cached.adoc[`cached` processor]"+` instead.

This processor will interpolate functions within the `+"`key` and `value`"+` fields individually for each message. This allows you to specify dynamic keys and values based on the contents of the message payloads and metadata. You can find a list of functions in xref:configuration:interpolation.adoc#bloblang-queries[Bloblang queries].`).
		Footnotes(`
== Operators

=== `+"`set`"+`

Set a key in the cache to a value. If the key already exists the contents are
overridden.

=== `+"`add`"+`

Set a key in the cache to a value. If the key already exists the action fails
with a 'key already exists' error, which can be detected with
xref:configuration:error_handling.adoc[processor error handling].

=== `+"`get`"+`

Retrieve the contents of a cached key and replace the original message payload
with the result. If the key does not exist the action fails with an error, which
can be detected with xref:configuration:error_handling.adoc[processor error handling].

=== `+"`delete`"+`

Delete a key and its contents from the cache. If the key does not exist the
action is a no-op and will not fail with an error.

=== `+"`exists`"+`

Check if a given key exists in the cache and replace the original message payload
with `+"`true`"+` or `+"`false`"+`.`).
		Example("Deduplication", `
Deduplication can be done using the add operator with a key extracted from the message payload, since it fails when a key already exists we can remove the duplicates using a xref:components:processors/mapping.adoc[`+"`mapping` processor"+`]:`,
			`
pipeline:
  processors:
    - cache:
        resource: foocache
        operator: add
        key: '${! json("message.id") }'
        value: "storeme"
    - mapping: root = if errored() { deleted() }

cache_resources:
  - label: foocache
    redis:
      url: tcp://TODO:6379
`).
		Example("Deduplication Batch-Wide", `
Sometimes it's necessary to deduplicate a batch of messages (also known as a window) by a single identifying value. This can be done by introducing a `+"xref:components:processors/branch.adoc[`branch` processor]"+`, which executes the cache only once on behalf of the batch, in this case with a value make from a field extracted from the first and last messages of the batch:`,
			`
pipeline:
  processors:
    # Try and add one message to a cache that identifies the whole batch
    - branch:
        request_map: |
          root = if batch_index() == 0 {
            json("id").from(0) + json("meta.tail_id").from(-1)
          } else { deleted() }
        processors:
          - cache:
              resource: foocache
              operator: add
              key: ${! content() }
              value: t
    # Delete all messages if we failed
    - mapping: |
        root = if errored().from(0) {
          deleted()
        }
`).
		Example("Hydration", `
It's possible to enrich payloads with content previously stored in a cache by using the xref:components:processors/branch.adoc[`+"`branch`"+`] processor:`,
			`
pipeline:
  processors:
    - branch:
        processors:
          - cache:
              resource: foocache
              operator: get
              key: '${! json("message.document_id") }'
        result_map: 'root.message.document = this'

        # NOTE: If the data stored in the cache is not valid JSON then use
        # something like this instead:
        # result_map: 'root.message.document = content().string()'

cache_resources:
  - label: foocache
    memcached:
      addresses: [ "TODO:11211" ]
`).
		Fields(
			service.NewStringField(cachePFieldResource).
				Description("The xref:components:caches/about.adoc[`cache` resource] to target with this processor."),
			service.NewStringEnumField(cachePFieldOperator, "set", "add", "get", "delete", "exists").
				Description("The <<operators, operation>> to perform with the cache."),
			service.NewInterpolatedStringField(cachePFieldKey).
				Description("A key to use with the cache."),
			service.NewInterpolatedStringField(cachePFieldValue).
				Description("A value to use with the cache (when applicable).").
				Optional(),
			service.NewInterpolatedStringField(cachePFieldTTL).
				Description("The TTL of each individual item as a duration string. After this period an item will be eligible for removal during the next compaction. Not all caches support per-key TTLs, those that do will have a configuration field `default_ttl`, and those that do not will fall back to their generally configured TTL setting.").
				Examples("60s", "5m", "36h").
				Version("3.33.0").
				Advanced().
				Optional(),
		)
}

type cacheProcConfig struct {
	Resource string
	Operator string
	Key      string
	Value    string
	TTL      string
}

func init() {
	service.MustRegisterBatchProcessor(
		"cache", cacheProcSpec(),
		func(conf *service.ParsedConfig, res *service.Resources) (service.BatchProcessor, error) {
			var cConf cacheProcConfig
			var err error

			if cConf.Resource, err = conf.FieldString(cachePFieldResource); err != nil {
				return nil, err
			}
			if cConf.Operator, err = conf.FieldString(cachePFieldOperator); err != nil {
				return nil, err
			}
			if cConf.Key, err = conf.FieldString(cachePFieldKey); err != nil {
				return nil, err
			}
			cConf.Value, _ = conf.FieldString(cachePFieldValue)
			cConf.TTL, _ = conf.FieldString(cachePFieldTTL)

			mgr := interop.UnwrapManagement(res)
			p, err := newCache(cConf, mgr)
			if err != nil {
				return nil, err
			}
			return interop.NewUnwrapInternalBatchProcessor(processor.NewAutoObservedBatchedProcessor("cache", p, mgr)), nil
		})
}

//------------------------------------------------------------------------------

type cacheProc struct {
	key   *field.Expression
	value *field.Expression
	ttl   *field.Expression

	mgr       bundle.NewManagement
	cacheName string
	operator  cacheOperator
}

func newCache(conf cacheProcConfig, mgr bundle.NewManagement) (*cacheProc, error) {
	cacheName := conf.Resource
	if cacheName == "" {
		return nil, errors.New("cache name must be specified")
	}

	op, err := cacheOperatorFromString(conf.Operator)
	if err != nil {
		return nil, err
	}

	key, err := mgr.BloblEnvironment().NewField(conf.Key)
	if err != nil {
		return nil, fmt.Errorf("failed to parse key expression: %v", err)
	}

	value, err := mgr.BloblEnvironment().NewField(conf.Value)
	if err != nil {
		return nil, fmt.Errorf("failed to parse value expression: %v", err)
	}

	ttl, err := mgr.BloblEnvironment().NewField(conf.TTL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ttl expression: %v", err)
	}

	if !mgr.ProbeCache(cacheName) {
		return nil, fmt.Errorf("cache resource '%v' was not found", cacheName)
	}

	return &cacheProc{
		key:   key,
		value: value,
		ttl:   ttl,

		mgr:       mgr,
		cacheName: cacheName,
		operator:  op,
	}, nil
}

//------------------------------------------------------------------------------

type (
	operatorResultApplier func(part *message.Part)
	cacheOperator         func(ctx context.Context, cache cache.V1, key string, value []byte, ttl *time.Duration) (operatorResultApplier, error)
)

func newCacheSetOperator() cacheOperator {
	return func(ctx context.Context, cache cache.V1, key string, value []byte, ttl *time.Duration) (operatorResultApplier, error) {
		err := cache.Set(ctx, key, value, ttl)
		return nil, err
	}
}

func newCacheAddOperator() cacheOperator {
	return func(ctx context.Context, cache cache.V1, key string, value []byte, ttl *time.Duration) (operatorResultApplier, error) {
		err := cache.Add(ctx, key, value, ttl)
		return nil, err
	}
}

func newCacheGetOperator() cacheOperator {
	return func(ctx context.Context, cache cache.V1, key string, _ []byte, _ *time.Duration) (operatorResultApplier, error) {
		result, err := cache.Get(ctx, key)
		return func(part *message.Part) { part.SetBytes(result) }, err
	}
}

func newCacheDeleteOperator() cacheOperator {
	return func(ctx context.Context, cache cache.V1, key string, _ []byte, ttl *time.Duration) (operatorResultApplier, error) {
		err := cache.Delete(ctx, key)
		return nil, err
	}
}

func newCacheExistsOperator() cacheOperator {
	return func(ctx context.Context, cache cache.V1, key string, _ []byte, _ *time.Duration) (operatorResultApplier, error) {
		if _, err := cache.Get(ctx, key); err != nil {
			return func(part *message.Part) { part.SetStructured(false) }, nil
		}

		return func(part *message.Part) { part.SetStructured(true) }, nil
	}
}

func cacheOperatorFromString(operator string) (cacheOperator, error) {
	switch operator {
	case "set":
		return newCacheSetOperator(), nil
	case "add":
		return newCacheAddOperator(), nil
	case "get":
		return newCacheGetOperator(), nil
	case "delete":
		return newCacheDeleteOperator(), nil
	case "exists":
		return newCacheExistsOperator(), nil
	}
	return nil, fmt.Errorf("operator not recognised: %v", operator)
}

//------------------------------------------------------------------------------

func (c *cacheProc) ProcessBatch(ctx *processor.BatchProcContext, msg message.Batch) ([]message.Batch, error) {
	_ = msg.Iter(func(index int, part *message.Part) error {
		key, err := c.key.String(index, msg)
		if err != nil {
			err = fmt.Errorf("key interpolation error: %w", err)
			ctx.OnError(err, index, nil)
			return nil
		}

		value, err := c.value.Bytes(index, msg)
		if err != nil {
			err = fmt.Errorf("value interpolation error: %w", err)
			ctx.OnError(err, index, nil)
			return nil
		}

		var ttl *time.Duration
		ttls, err := c.ttl.String(index, msg)
		if err != nil {
			err = fmt.Errorf("ttl interpolation error: %w", err)
			ctx.OnError(err, index, nil)
			return nil
		}

		if ttls != "" {
			td, err := time.ParseDuration(ttls)
			if err != nil {
				err = fmt.Errorf("ttl must be a duration: %w", err)
				ctx.OnError(err, index, nil)
				return nil
			}
			ttl = &td
		}

		var resultApplierFn operatorResultApplier
		if cerr := c.mgr.AccessCache(context.Background(), c.cacheName, func(cache cache.V1) {
			resultApplierFn, err = c.operator(context.Background(), cache, key, value, ttl)
		}); cerr != nil {
			err = cerr
		}
		if err != nil {
			if err != component.ErrKeyAlreadyExists {
				err = fmt.Errorf("operator failed for key '%s': %v", key, err)
			} else {
				err = fmt.Errorf("key already exists: %v", key)
			}
			ctx.OnError(err, index, nil)
			return nil
		}

		if resultApplierFn != nil {
			resultApplierFn(part)
		}
		return nil
	})

	return []message.Batch{msg}, nil
}

func (c *cacheProc) Close(ctx context.Context) error {
	return nil
}
