// Copyright 2025 Redpanda Data, Inc.

package query

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/Jeffail/gabs/v2"
	"github.com/gofrs/uuid/v5"
	gonanoid "github.com/matoous/go-nanoid/v2"
	"github.com/segmentio/ksuid"

	"github.com/redpanda-data/benthos/v4/internal/tracing"
	"github.com/redpanda-data/benthos/v4/internal/value"
)

type fieldFunction struct {
	namedContext string
	fromRoot     bool
	path         []string
}

func (f *fieldFunction) expand(path ...string) *fieldFunction {
	newFn := *f
	newPath := make([]string, 0, len(f.path)+len(path))
	newPath = append(newPath, f.path...)
	newPath = append(newPath, path...)
	newFn.path = newPath
	return &newFn
}

func (f *fieldFunction) Annotation() string {
	path := f.namedContext
	if f.fromRoot {
		path = "root"
	} else if path == "" {
		path = "this"
	}
	if len(f.path) > 0 {
		path = path + "." + SliceToDotPath(f.path...)
	}
	return "field `" + path + "`"
}

func (f *fieldFunction) Exec(ctx FunctionContext) (any, error) {
	var target any
	if f.fromRoot {
		if ctx.NewValue == nil {
			return nil, errors.New("unable to reference `root` from this context")
		}
		target = *ctx.NewValue
	} else if f.namedContext == "" {
		v := ctx.Value()
		if v == nil {
			var fieldName string
			if len(f.path) > 0 {
				fieldName = SliceToDotPath(f.path...)
			}
			return nil, ErrNoContext{
				FieldName: fieldName,
			}
		}
		target = *v
	} else {
		var ok bool
		if target, ok = ctx.NamedValue(f.namedContext); !ok {
			return ctx, fmt.Errorf("named context %v was not found", f.namedContext)
		}
	}
	if len(f.path) == 0 {
		return target, nil
	}
	return gabs.Wrap(target).S(f.path...).Data(), nil
}

func (f *fieldFunction) QueryTargets(ctx TargetsContext) (TargetsContext, []TargetPath) {
	var basePaths []TargetPath
	if f.fromRoot {
		basePaths = []TargetPath{NewTargetPath(TargetRoot)}
	} else if f.namedContext == "" {
		if basePaths = ctx.MainContext(); len(basePaths) == 0 {
			basePaths = []TargetPath{NewTargetPath(TargetValue)}
		}
	} else {
		basePaths = ctx.NamedContext(f.namedContext)
	}
	paths := make([]TargetPath, len(basePaths))
	for i, p := range basePaths {
		paths[i] = p
		paths[i].Path = append(paths[i].Path, f.path...)
	}
	ctx = ctx.WithValues(paths)
	return ctx, paths
}

func (f *fieldFunction) Close(ctx context.Context) error {
	return nil
}

// NewNamedContextFieldFunction creates a query function that attempts to
// return a field from a named context.
func NewNamedContextFieldFunction(namedContext, pathStr string) Function {
	var path []string
	if pathStr != "" {
		path = gabs.DotPathToSlice(pathStr)
	}
	return &fieldFunction{namedContext: namedContext, fromRoot: false, path: path}
}

// NewFieldFunction creates a query function that returns a field from the
// current context.
func NewFieldFunction(pathStr string) Function {
	var path []string
	if pathStr != "" {
		path = gabs.DotPathToSlice(pathStr)
	}
	return &fieldFunction{
		path: path,
	}
}

// NewRootFieldFunction creates a query function that returns a field from the
// root context.
func NewRootFieldFunction(pathStr string) Function {
	var path []string
	if pathStr != "" {
		path = gabs.DotPathToSlice(pathStr)
	}
	return &fieldFunction{
		fromRoot: true,
		path:     path,
	}
}

//------------------------------------------------------------------------------

// Literal wraps a static value and returns it for each invocation of the
// function.
type Literal struct {
	annotation string
	Value      any
}

// Annotation returns a token identifier of the function.
func (l *Literal) Annotation() string {
	if l.annotation == "" {
		return string(value.ITypeOf(l.Value)) + " literal"
	}
	return l.annotation
}

// Exec returns a literal value.
func (l *Literal) Exec(ctx FunctionContext) (any, error) {
	return l.Value, nil
}

// QueryTargets returns nothing.
func (l *Literal) QueryTargets(ctx TargetsContext) (TargetsContext, []TargetPath) {
	return ctx, nil
}

// Close does nothing.
func (l *Literal) Close(ctx context.Context) error {
	return nil
}

// String returns a string representation of the literal function.
func (l *Literal) String() string {
	return fmt.Sprintf("%v", l.Value)
}

// NewLiteralFunction creates a query function that returns a static, literal
// value.
func NewLiteralFunction(annotation string, v any) *Literal {
	return &Literal{annotation: annotation, Value: v}
}

//------------------------------------------------------------------------------

var _ = registerSimpleFunction(
	NewFunctionSpec(
		FunctionCategoryMessage, "batch_index",
		"Returns the zero-based index of the current message within its batch. Use this to conditionally process messages based on their position, or to create sequential identifiers within a batch.",
		NewExampleSpec("",
			`root = if batch_index() > 0 { deleted() }`,
		),
		NewExampleSpec("Create a unique identifier combining batch position with timestamp.",
			`root.id = "%v-%v".format(timestamp_unix(), batch_index())`,
		),
	),
	func(ctx FunctionContext) (any, error) {
		return int64(ctx.Index), nil
	},
)

//------------------------------------------------------------------------------

var _ = registerSimpleFunction(
	NewFunctionSpec(
		FunctionCategoryMessage, "batch_size",
		"Returns the total number of messages in the current batch. Use this to determine batch boundaries or compute relative positions.",
		NewExampleSpec("",
			`root.total = batch_size()`,
		),
		NewExampleSpec("Check if processing the last message in a batch.",
			`root.is_last = batch_index() == batch_size() - 1`,
		),
	),
	func(ctx FunctionContext) (any, error) {
		return int64(ctx.MsgBatch.Len()), nil
	},
)

//------------------------------------------------------------------------------

var _ = registerSimpleFunction(
	NewFunctionSpec(
		FunctionCategoryMessage, "content",
		"Returns the raw message payload as bytes, regardless of the current mapping context. Use this to access the original message when working within nested contexts, or to store the entire message as a field.",
		NewExampleSpec("",
			`root.doc = content().string()`,
			`{"foo":"bar"}`,
			`{"doc":"{\"foo\":\"bar\"}"}`,
		),
		NewExampleSpec("Preserve original message while adding metadata.",
			`root.original = content().string()
root.processed_by = "ai"`,
			`{"foo":"bar"}`,
			`{"original":"{\"foo\":\"bar\"}","processed_by":"ai"}`,
		),
	),
	func(ctx FunctionContext) (any, error) {
		return ctx.MsgBatch.Get(ctx.Index).AsBytes(), nil
	},
)

//------------------------------------------------------------------------------

var _ = registerSimpleFunction(
	NewFunctionSpec(
		FunctionCategoryMessage, "tracing_span",
		"Returns the OpenTelemetry tracing span attached to the message as a text map object, or `null` if no span exists. Use this to propagate trace context to downstream systems via headers or metadata.",
		NewExampleSpec("",
			`root.headers.traceparent = tracing_span().traceparent`,
			`{"some_stuff":"just can't be explained by science"}`,
			`{"headers":{"traceparent":"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"}}`,
		),
		NewExampleSpec("Forward all tracing fields to output metadata.",
			`meta = tracing_span()`,
		),
	).Experimental(),
	func(fCtx FunctionContext) (any, error) {
		span := tracing.GetSpan(fCtx.MsgBatch.Get(fCtx.Index))
		if span == nil {
			return nil, nil
		}
		return span.TextMap()
	},
)

//------------------------------------------------------------------------------

var _ = registerSimpleFunction(
	NewFunctionSpec(
		FunctionCategoryMessage, "tracing_id",
		"Returns the OpenTelemetry trace ID for the message, or an empty string if no tracing span exists. Use this to correlate logs and events with distributed traces.",
		NewExampleSpec("",
			`meta trace_id = tracing_id()`,
		),
		NewExampleSpec("Add trace ID to structured logs.",
			`root.log_entry = this
root.log_entry.trace_id = tracing_id()`,
		),
	).Experimental(),
	func(fCtx FunctionContext) (any, error) {
		traceID := tracing.GetTraceID(fCtx.MsgBatch.Get(fCtx.Index))
		return traceID, nil
	},
)

//------------------------------------------------------------------------------

var _ = registerFunction(
	NewDeprecatedFunctionSpec(
		"count",
		"The `count` function is a counter starting at 1 which increments after each time it is called. Count takes an argument which is an identifier for the counter, allowing you to specify multiple unique counters in your configuration.",
		NewExampleSpec("",
			`root = this
root.id = count("bloblang_function_example")`,
			`{"message":"foo"}`,
			`{"id":1,"message":"foo"}`,
			`{"message":"bar"}`,
			`{"id":2,"message":"bar"}`,
		),
	).Param(ParamString("name", "An identifier for the counter.")).MarkImpure(),
	countFunction,
)

var (
	counters    = map[string]int64{}
	countersMux = &sync.Mutex{}
)

func countFunction(args *ParsedParams) (Function, error) {
	name, err := args.FieldString("name")
	if err != nil {
		return nil, err
	}
	return ClosureFunction("function count", func(ctx FunctionContext) (any, error) {
		countersMux.Lock()
		defer countersMux.Unlock()

		var count int64
		var exists bool

		if count, exists = counters[name]; exists {
			count++
		} else {
			count = 1
		}
		counters[name] = count

		return count, nil
	}, nil), nil
}

//------------------------------------------------------------------------------

var _ = registerFunction(
	NewFunctionSpec(
		FunctionCategoryGeneral, "deleted",
		"Returns a deletion marker that removes the target field or message. When applied to root, the entire message is dropped while still being acknowledged as successfully processed. Use this to filter data or conditionally remove fields.",
		NewExampleSpec("",
			`root = this
root.bar = deleted()`,
			`{"bar":"bar_value","baz":"baz_value","foo":"foo value"}`,
			`{"baz":"baz_value","foo":"foo value"}`,
		),
		NewExampleSpec(
			"Filter array elements by returning deleted for unwanted items.",
			`root.new_nums = this.nums.map_each(num -> if num < 10 { deleted() } else { num - 10 })`,
			`{"nums":[3,11,4,17]}`,
			`{"new_nums":[1,7]}`,
		),
	),
	func(*ParsedParams) (Function, error) {
		return NewLiteralFunction("delete", value.Delete(nil)), nil
	},
)

//------------------------------------------------------------------------------

var _ = registerSimpleFunction(
	NewFunctionSpec(
		FunctionCategoryMessage, "error",
		"Returns the error message string if the message has failed processing, otherwise `null`. Use this in error handling pipelines to log or route failed messages based on their error details.",
		NewExampleSpec("",
			`root.doc.error = error()`,
		),
		NewExampleSpec("Route messages to different outputs based on error presence.",
			`root = this
root.error_msg = error()
root.has_error = error() != null`,
		),
	),
	func(ctx FunctionContext) (any, error) {
		if err := ctx.MsgBatch.Get(ctx.Index).ErrorGet(); err != nil {
			return err.Error(), nil
		}
		return nil, nil
	},
)

var _ = registerSimpleFunction(
	NewFunctionSpec(
		FunctionCategoryMessage, "errored",
		"Returns true if the message has failed processing, false otherwise. Use this for conditional logic in error handling workflows or to route failed messages to dead letter queues.",
		NewExampleSpec("",
			`root.doc.status = if errored() { 400 } else { 200 }`,
		),
		NewExampleSpec("Send only failed messages to a separate stream.",
			`root = if errored() { this } else { deleted() }`,
		),
	),
	func(ctx FunctionContext) (any, error) {
		return ctx.MsgBatch.Get(ctx.Index).ErrorGet() != nil, nil
	},
)

var _ = registerSimpleFunction(
	NewFunctionSpec(
		FunctionCategoryMessage, "error_source_name",
		"Returns the component name that caused the error, or `null` if the message has no error or the error has no associated component. Use this to identify which processor or component in your pipeline caused a failure.",
		NewExampleSpec("",
			`root.doc.error_source_name = error_source_name()`,
		),
		NewExampleSpec("Create detailed error logs with component information.",
			`root.error_details = if errored() {
  {
    "message": error(),
    "component": error_source_name(),
    "timestamp": now()
  }
}`,
		),
	),
	func(ctx FunctionContext) (any, error) {
		if err := ctx.MsgBatch.Get(ctx.Index).ErrorGet(); err != nil {
			if cErr, ok := err.(*ComponentError); ok {
				return cErr.Name, nil
			}
		}
		return nil, nil
	},
)

var _ = registerSimpleFunction(
	NewFunctionSpec(
		FunctionCategoryMessage, "error_source_label",
		"Returns the user-defined label of the component that caused the error, empty string if no label is set, or `null` if the message has no error. Use this for more human-readable error tracking when components have custom labels.",
		NewExampleSpec("",
			`root.doc.error_source_label = error_source_label()`,
		),
		NewExampleSpec("Route errors based on component labels.",
			`root.error_category = error_source_label().or("unknown")`,
		),
	),
	func(ctx FunctionContext) (any, error) {
		if err := ctx.MsgBatch.Get(ctx.Index).ErrorGet(); err != nil {
			if cErr, ok := err.(*ComponentError); ok {
				return cErr.Label, nil
			}
		}
		return nil, nil
	},
)

var _ = registerSimpleFunction(
	NewFunctionSpec(
		FunctionCategoryMessage, "error_source_path",
		"Returns the dot-separated path to the component that caused the error, or `null` if the message has no error. Use this to identify the exact location of a failed component in nested pipeline configurations.",
		NewExampleSpec("",
			`root.doc.error_source_path = error_source_path()`,
		),
		NewExampleSpec("Build comprehensive error context for debugging.",
			`root.error_info = {
  "path": error_source_path(),
  "component": error_source_name(),
  "message": error()
}`,
		),
	),
	func(ctx FunctionContext) (any, error) {
		if err := ctx.MsgBatch.Get(ctx.Index).ErrorGet(); err != nil {
			if cErr, ok := err.(*ComponentError); ok {
				return SliceToDotPath(cErr.Path...), nil
			}
		}
		return nil, nil
	},
)

//------------------------------------------------------------------------------

var _ = registerFunction(
	NewFunctionSpec(
		FunctionCategoryGeneral, "range",
		"Creates an array of integers from start (inclusive) to stop (exclusive) with an optional step. Use this to generate sequences for iteration, indexing, or creating numbered lists.",
		NewExampleSpec("",
			`root.a = range(0, 10)
root.b = range(start: 0, stop: this.max, step: 2) # Using named params
root.c = range(0, -this.max, -2)`,
			`{"max":10}`,
			`{"a":[0,1,2,3,4,5,6,7,8,9],"b":[0,2,4,6,8],"c":[0,-2,-4,-6,-8]}`,
		),
		NewExampleSpec("Generate a sequence for batch processing.",
			`root.pages = range(0, this.total_items, 100).map_each(offset -> {
  "offset": offset,
  "limit": 100
})`,
			`{"total_items":250}`,
			`{"pages":[{"limit":100,"offset":0},{"limit":100,"offset":100}]}`,
		),
	).
		Param(ParamInt64("start", "The start value.")).
		Param(ParamInt64("stop", "The stop value.")).
		Param(ParamInt64("step", "The step value.").Default(1)),
	rangeFunction,
)

func rangeFunction(args *ParsedParams) (Function, error) {
	start, err := args.FieldInt64("start")
	if err != nil {
		return nil, err
	}
	stop, err := args.FieldInt64("stop")
	if err != nil {
		return nil, err
	}
	step, err := args.FieldInt64("step")
	if err != nil {
		return nil, err
	}
	if step == 0 {
		return nil, errors.New("step must be greater than or less than 0")
	}
	if step < 0 {
		if stop > start {
			return nil, fmt.Errorf("with negative step arg stop (%v) must be <= start (%v)", stop, start)
		}
	} else if start >= stop {
		return nil, fmt.Errorf("with positive step arg start (%v) must be < stop (%v)", start, stop)
	}
	r := make([]any, (stop-start)/step)
	for i := 0; i < len(r); i++ {
		r[i] = start + step*int64(i)
	}
	return ClosureFunction("function range", func(ctx FunctionContext) (any, error) {
		return r, nil
	}, nil), nil
}

var _ = registerFunction(
	NewFunctionSpec(
		FunctionCategoryMessage, "json",
		"Returns a field from the original JSON message by dot path, always accessing the root document regardless of mapping context. Use this to reference the source message when working in nested contexts or to extract specific fields.",
		NewExampleSpec("",
			`root.mapped = json("foo.bar")`,
			`{"foo":{"bar":"hello world"}}`,
			`{"mapped":"hello world"}`,
		),
		NewExampleSpec(
			"Access the original message from within nested mapping contexts.",
			`root.doc = json()`,
			`{"foo":{"bar":"hello world"}}`,
			`{"doc":{"foo":{"bar":"hello world"}}}`,
		),
	).Param(ParamString("path", "An optional [dot path][field_paths] identifying a field to obtain.").Default("")),
	jsonFunction,
)

func jsonFunction(args *ParsedParams) (Function, error) {
	path, err := args.FieldString("path")
	if err != nil {
		return nil, err
	}
	var argPath []string
	if path != "" {
		argPath = gabs.DotPathToSlice(path)
	}
	return ClosureFunction("json path `"+SliceToDotPath(argPath...)+"`", func(ctx FunctionContext) (any, error) {
		jPart, err := ctx.MsgBatch.Get(ctx.Index).AsStructured()
		if err != nil {
			return nil, err
		}
		gPart := gabs.Wrap(jPart)
		if len(argPath) > 0 {
			gPart = gPart.Search(argPath...)
		}
		return value.ISanitize(gPart.Data()), nil
	}, func(ctx TargetsContext) (TargetsContext, []TargetPath) {
		paths := []TargetPath{
			NewTargetPath(TargetValue, argPath...),
		}
		ctx = ctx.WithValues(paths)
		return ctx, paths
	}), nil
}

//------------------------------------------------------------------------------

// NewMetaFunction creates a new function for obtaining a metadata value.
func NewMetaFunction(key string) Function {
	if key != "" {
		return ClosureFunction("meta field "+key, func(ctx FunctionContext) (any, error) {
			if ctx.NewMeta == nil {
				return nil, errors.New("metadata cannot be queried in this context")
			}
			v, exists := ctx.NewMeta.MetaGetMut(key)
			if !exists {
				return nil, nil
			}
			return v, nil
		}, func(ctx TargetsContext) (TargetsContext, []TargetPath) {
			paths := []TargetPath{
				NewTargetPath(TargetMetadata, key),
			}
			ctx = ctx.WithValues(paths)
			return ctx, paths
		})
	}
	return ClosureFunction("meta object", func(ctx FunctionContext) (any, error) {
		if ctx.NewMeta == nil {
			return nil, errors.New("metadata cannot be queried in this context")
		}
		kvs := map[string]any{}
		_ = ctx.NewMeta.MetaIterMut(func(k string, v any) error {
			kvs[k] = v
			return nil
		})
		return kvs, nil
	}, func(ctx TargetsContext) (TargetsContext, []TargetPath) {
		paths := []TargetPath{
			NewTargetPath(TargetMetadata),
		}
		ctx = ctx.WithValues(paths)
		return ctx, paths
	})
}

var _ = registerFunction(
	NewFunctionSpec(
		FunctionCategoryMessage, "metadata",
		"Returns metadata from the input message by key, or `null` if the key doesn't exist. This reads the original metadata; to access modified metadata during mapping, use the `@` operator instead. Use this to extract message properties like topics, headers, or timestamps.",
		NewExampleSpec("", `root.topic = metadata("kafka_topic")`),
		NewExampleSpec(
			"Retrieve all metadata as an object by omitting the key parameter.",
			`root.all_metadata = metadata()`,
		),
		NewExampleSpec(
			"Copy specific metadata fields to the message body.",
			`root.source = {
  "topic": metadata("kafka_topic"),
  "partition": metadata("kafka_partition"),
  "timestamp": metadata("kafka_timestamp_unix")
}`,
		),
	).Param(ParamString("key", "An optional key of a metadata value to obtain.").Default("")),
	func(args *ParsedParams) (Function, error) {
		key, err := args.FieldString("key")
		if err != nil {
			return nil, err
		}
		if key != "" {
			return ClosureFunction("metadata field "+key, func(ctx FunctionContext) (any, error) {
				v, exists := ctx.MsgBatch.Get(ctx.Index).MetaGetMut(key)
				if !exists {
					return nil, nil
				}
				return v, nil
			}, func(ctx TargetsContext) (TargetsContext, []TargetPath) {
				paths := []TargetPath{
					NewTargetPath(TargetMetadata, key),
				}
				ctx = ctx.WithValues(paths)
				return ctx, paths
			}), nil
		}
		return ClosureFunction("metadata object", func(ctx FunctionContext) (any, error) {
			kvs := map[string]any{}
			_ = ctx.MsgBatch.Get(ctx.Index).MetaIterMut(func(k string, v any) error {
				kvs[k] = v
				return nil
			})
			return kvs, nil
		}, func(ctx TargetsContext) (TargetsContext, []TargetPath) {
			paths := []TargetPath{
				NewTargetPath(TargetMetadata),
			}
			ctx = ctx.WithValues(paths)
			return ctx, paths
		}), nil
	},
)

var _ = registerFunction(
	NewDeprecatedFunctionSpec(
		"meta",
		"Returns the value of a metadata key from the input message as a string, or `null` if the key does not exist. Since values are extracted from the read-only input message they do NOT reflect changes made from within the map. In order to query metadata mutations made within a mapping use the <<root_meta, `root_meta` function>>. This function supports extracting metadata from other messages of a batch with the `from` method.",
		NewExampleSpec("",
			`root.topic = meta("kafka_topic")`,
			`root.topic = meta("nope") | meta("also nope") | "default"`,
		),
		NewExampleSpec(
			"The key parameter is optional and if omitted the entire metadata contents are returned as an object.",
			`root.all_metadata = meta()`,
		),
	).Param(ParamString("key", "An optional key of a metadata value to obtain.").Default("")),
	func(args *ParsedParams) (Function, error) {
		key, err := args.FieldString("key")
		if err != nil {
			return nil, err
		}
		if key != "" {
			return ClosureFunction("meta field "+key, func(ctx FunctionContext) (any, error) {
				v := ctx.MsgBatch.Get(ctx.Index).MetaGetStr(key)
				if v == "" {
					return nil, nil
				}
				return v, nil
			}, func(ctx TargetsContext) (TargetsContext, []TargetPath) {
				paths := []TargetPath{
					NewTargetPath(TargetMetadata, key),
				}
				ctx = ctx.WithValues(paths)
				return ctx, paths
			}), nil
		}
		return ClosureFunction("meta object", func(ctx FunctionContext) (any, error) {
			kvs := map[string]any{}
			_ = ctx.MsgBatch.Get(ctx.Index).MetaIterStr(func(k, v string) error {
				kvs[k] = v
				return nil
			})
			return kvs, nil
		}, func(ctx TargetsContext) (TargetsContext, []TargetPath) {
			paths := []TargetPath{
				NewTargetPath(TargetMetadata),
			}
			ctx = ctx.WithValues(paths)
			return ctx, paths
		}), nil
	},
)

//------------------------------------------------------------------------------

var _ = registerFunction(
	NewDeprecatedFunctionSpec(
		"root_meta",
		"Returns the value of a metadata key from the new message being created as a string, or `null` if the key does not exist. Changes made to metadata during a mapping will be reflected by this function.",
		NewExampleSpec("",
			`root.topic = root_meta("kafka_topic")`,
			`root.topic = root_meta("nope") | root_meta("also nope") | "default"`,
		),
		NewExampleSpec(
			"The key parameter is optional and if omitted the entire metadata contents are returned as an object.",
			`root.all_metadata = root_meta()`,
		),
	).Param(ParamString("key", "An optional key of a metadata value to obtain.").Default("")),
	func(args *ParsedParams) (Function, error) {
		key, err := args.FieldString("key")
		if err != nil {
			return nil, err
		}
		if key != "" {
			return ClosureFunction("root_meta field "+key, func(ctx FunctionContext) (any, error) {
				if ctx.NewMeta == nil {
					return nil, errors.New("root metadata cannot be queried in this context")
				}
				v := ctx.NewMeta.MetaGetStr(key)
				if v == "" {
					return nil, nil
				}
				return v, nil
			}, func(ctx TargetsContext) (TargetsContext, []TargetPath) {
				paths := []TargetPath{
					NewTargetPath(TargetMetadata, key),
				}
				ctx = ctx.WithValues(paths)
				return ctx, paths
			}), nil
		}
		return ClosureFunction("root_meta object", func(ctx FunctionContext) (any, error) {
			if ctx.NewMeta == nil {
				return nil, errors.New("root metadata cannot be queried in this context")
			}
			kvs := map[string]any{}
			_ = ctx.NewMeta.MetaIterStr(func(k, v string) error {
				kvs[k] = v
				return nil
			})
			return kvs, nil
		}, func(ctx TargetsContext) (TargetsContext, []TargetPath) {
			paths := []TargetPath{
				NewTargetPath(TargetMetadata),
			}
			ctx = ctx.WithValues(paths)
			return ctx, paths
		}), nil
	},
)

//------------------------------------------------------------------------------

var _ = registerFunction(
	NewHiddenFunctionSpec("nothing"),
	func(*ParsedParams) (Function, error) {
		return NewLiteralFunction("nothing", value.Nothing(nil)), nil
	},
)

//------------------------------------------------------------------------------

var _ = registerFunction(
	NewFunctionSpec(
		FunctionCategoryGeneral, "random_int", `
Generates a pseudo-random non-negative 64-bit integer. Use this for creating random IDs, sampling data, or generating test values. Provide a seed for reproducible randomness, or use a dynamic seed like `+"`timestamp_unix_nano()`"+` for unique values per mapping instance.

Optional `+"`min` and `max`"+` parameters constrain the output range (both inclusive). For dynamic ranges based on message data, use the modulo operator instead: `+"`random_int() % dynamic_max + dynamic_min`"+`.`,
		NewExampleSpec("",
			`root.first = random_int()
root.second = random_int(1)
root.third = random_int(max:20)
root.fourth = random_int(min:10, max:20)
root.fifth = random_int(timestamp_unix_nano(), 5, 20)
root.sixth = random_int(seed:timestamp_unix_nano(), max:20)
`,
		),
		NewExampleSpec("Use a dynamic seed for unique random values per mapping instance.",
			`root.random_id = random_int(timestamp_unix_nano())
root.sample_percent = random_int(seed: timestamp_unix_nano(), min: 0, max: 100)`,
		),
	).
		Param(ParamQuery(
			"seed",
			"A seed to use, if a query is provided it will only be resolved once during the lifetime of the mapping.",
			true,
		).Default(NewLiteralFunction("", 0))).
		Param(ParamInt64("min", "The minimum value the random generated number will have. The default value is 0.").Default(0).DisableDynamic()).
		Param(ParamInt64("max", fmt.Sprintf("The maximum value the random generated number will have. The default value is %d (math.MaxInt64 - 1).", uint64(math.MaxInt64-1))).Default(int64(math.MaxInt64-1)).DisableDynamic()),
	randomIntFunction,
)

func randomIntFunction(args *ParsedParams) (Function, error) {
	seedFn, err := args.FieldQuery("seed")
	if err != nil {
		return nil, err
	}
	minV, err := args.FieldInt64("min")
	if err != nil {
		return nil, err
	}
	maxV, err := args.FieldInt64("max")
	if err != nil {
		return nil, err
	}
	if minV < 0 {
		return nil, fmt.Errorf("min (%d) must be a positive number", minV)
	}
	if maxV < minV {
		return nil, fmt.Errorf("min (%d) must be smaller or equal than max (%d)", minV, maxV)
	}
	if maxV == math.MaxInt64 {
		return nil, fmt.Errorf("max must be smaller than the max allowed for an int64 (%d)", uint64(math.MaxInt64))
	}
	var randMut sync.Mutex
	var r *rand.Rand

	return ClosureFunction("function random_int", func(ctx FunctionContext) (any, error) {
		randMut.Lock()
		defer randMut.Unlock()

		if r == nil {
			seedI, err := seedFn.Exec(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to seed random number generator: %v", err)
			}

			seed, err := value.IToInt(seedI)
			if err != nil {
				return nil, fmt.Errorf("failed to seed random number generator: %v", err)
			}

			r = rand.New(rand.NewSource(seed))
		}
		// Int63n generates a random number within a half-open interval [0,n)
		v := r.Int63n(maxV-minV+1) + minV
		return v, nil
	}, nil), nil
}

//------------------------------------------------------------------------------

var _ = registerFunction(
	NewFunctionSpec(
		FunctionCategoryEnvironment, "now",
		"Returns the current timestamp as an RFC 3339 formatted string with nanosecond precision. Use this to add processing timestamps to messages or measure time between events. Chain with `ts_format` to customize the format or timezone.",
		NewExampleSpec("",
			`root.received_at = now()`,
		),
		NewExampleSpec("Format the timestamp in a custom format and timezone.",
			`root.received_at = now().ts_format("Mon Jan 2 15:04:05 -0700 MST 2006", "UTC")`,
		),
	),
	func(args *ParsedParams) (Function, error) {
		return ClosureFunction("function now", func(_ FunctionContext) (any, error) {
			return time.Now().Format(time.RFC3339Nano), nil
		}, nil), nil
	},
)

var _ = registerSimpleFunction(
	NewFunctionSpec(
		FunctionCategoryEnvironment, "timestamp_unix",
		"Returns the current Unix timestamp in seconds since epoch. Use this for numeric timestamps compatible with most systems, or as a seed for random number generation.",
		NewExampleSpec("",
			`root.received_at = timestamp_unix()`,
		),
		NewExampleSpec("Create a sortable ID combining timestamp with a counter.",
			`root.id = "%v-%v".format(timestamp_unix(), batch_index())`,
		),
	),
	func(_ FunctionContext) (any, error) {
		return time.Now().Unix(), nil
	},
)

var _ = registerSimpleFunction(
	NewFunctionSpec(
		FunctionCategoryEnvironment, "timestamp_unix_milli",
		"Returns the current Unix timestamp in milliseconds since epoch. Use this for millisecond-precision timestamps common in web APIs and JavaScript systems.",
		NewExampleSpec("",
			`root.received_at = timestamp_unix_milli()`,
		),
		NewExampleSpec("Add processing time metadata.",
			`meta processing_time_ms = timestamp_unix_milli()`,
		),
	),
	func(_ FunctionContext) (any, error) {
		return time.Now().UnixMilli(), nil
	},
)

var _ = registerSimpleFunction(
	NewFunctionSpec(
		FunctionCategoryEnvironment, "timestamp_unix_micro",
		"Returns the current Unix timestamp in microseconds since epoch. Use this for high-precision timing measurements or when microsecond resolution is required.",
		NewExampleSpec("",
			`root.received_at = timestamp_unix_micro()`,
		),
		NewExampleSpec("Measure elapsed time between events.",
			`root.processing_duration_us = timestamp_unix_micro() - this.start_time_us`,
		),
	),
	func(_ FunctionContext) (any, error) {
		return time.Now().UnixMicro(), nil
	},
)

var _ = registerSimpleFunction(
	NewFunctionSpec(
		FunctionCategoryEnvironment, "timestamp_unix_nano",
		"Returns the current Unix timestamp in nanoseconds since epoch. Use this for the highest precision timing or as a unique seed value that changes on every invocation.",
		NewExampleSpec("",
			`root.received_at = timestamp_unix_nano()`,
		),
		NewExampleSpec("Generate unique random values on each mapping.",
			`root.random_value = random_int(timestamp_unix_nano())`,
		),
	),
	func(_ FunctionContext) (any, error) {
		return time.Now().UnixNano(), nil
	},
)

//------------------------------------------------------------------------------

var _ = registerFunction(
	NewFunctionSpec(
		FunctionCategoryGeneral, "throw",
		"Immediately fails the mapping with a custom error message. Use this to halt processing when data validation fails or required fields are missing, causing the message to be routed to error handlers.",
		NewExampleSpec("",
			`root.doc.type = match {
  this.exists("header.id") => "foo"
  this.exists("body.data") => "bar"
  _ => throw("unknown type")
}
root.doc.contents = (this.body.content | this.thing.body)`,
			`{"header":{"id":"first"},"thing":{"body":"hello world"}}`,
			`{"doc":{"contents":"hello world","type":"foo"}}`,
			`{"nothing":"matches"}`,
			`Error("failed assignment (line 1): unknown type")`,
		),
		NewExampleSpec("Validate required fields before processing.",
			`root = if this.exists("user_id") {
  this
} else {
  throw("missing required field: user_id")
}`,
			`{"user_id":123,"name":"alice"}`,
			`{"name":"alice","user_id":123}`,
			`{"name":"bob"}`,
			`Error("failed assignment (line 1): missing required field: user_id")`,
		),
	).Param(ParamString("why", "A string explanation for why an error was thrown, this will be added to the resulting error message.")),
	func(args *ParsedParams) (Function, error) {
		msg, err := args.FieldString("why")
		if err != nil {
			return nil, err
		}
		return ClosureFunction("function throw", func(_ FunctionContext) (any, error) {
			return nil, errors.New(msg)
		}, nil), nil
	},
)

//------------------------------------------------------------------------------

var _ = registerSimpleFunction(
	NewFunctionSpec(
		FunctionCategoryGeneral, "uuid_v4",
		"Generates a random RFC-4122 version 4 UUID. Use this for creating unique identifiers that don't reveal timing information or require ordering. Each invocation produces a new globally unique ID.",
		NewExampleSpec("", `root.id = uuid_v4()`),
		NewExampleSpec("Add unique request IDs for tracing.",
			`root = this
root.request_id = uuid_v4()`,
		),
	),
	func(_ FunctionContext) (any, error) {
		u4, err := uuid.NewV4()
		if err != nil {
			panic(err)
		}
		return u4.String(), nil
	},
)

var _ = registerFunction(
	NewFunctionSpec(
		FunctionCategoryGeneral, "uuid_v7",
		"Generates a time-ordered UUID version 7 with millisecond timestamp precision. Use this for sortable unique identifiers that maintain chronological ordering, ideal for database keys or event IDs. Optionally specify a custom timestamp.",
		NewExampleSpec("", `root.id = uuid_v7()`),
		NewExampleSpec("Generate a UUID with a specific timestamp for backdating events.",
			`root.id = uuid_v7(now().ts_sub_iso8601("PT1M"))`,
		),
	).Param(ParamTimestamp("time", "An optional timestamp to use for the time ordered portion of the UUID.").Optional()),
	func(args *ParsedParams) (Function, error) {
		time, err := args.FieldOptionalTimestamp("time")
		if err != nil {
			return nil, err
		}
		return ClosureFunction("function uuid_v7", func(fctx FunctionContext) (any, error) {
			if time == nil {
				u7, err := uuid.NewV7()
				if err != nil {
					return nil, fmt.Errorf("unable to generate uuid v7: %w", err)
				}
				return u7.String(), nil
			}
			u7, err := uuid.NewV7AtTime(*time)
			if err != nil {
				return nil, fmt.Errorf("unable to generate uuid v7 at time %s: %w", *time, err)
			}
			return u7.String(), nil
		}, nil), nil
	},
)

//------------------------------------------------------------------------------

var _ = registerFunction(
	NewFunctionSpec(
		FunctionCategoryGeneral, "nanoid",
		"Generates a URL-safe unique identifier using Nano ID. Use this for compact, URL-friendly IDs with good collision resistance. Customize the length (default 21) or provide a custom alphabet for specific character requirements.",
		NewExampleSpec("", `root.id = nanoid()`),
		NewExampleSpec("Generate a longer ID for additional uniqueness.", `root.id = nanoid(54)`),
		NewExampleSpec("Use a custom alphabet for domain-specific IDs.", `root.id = nanoid(54, "abcde")`),
	).
		Param(ParamInt64("length", "An optional length.").Optional()).
		Param(ParamString("alphabet", "An optional custom alphabet to use for generating IDs. When specified the field `length` must also be present.").Optional()),
	nanoidFunction,
)

func nanoidFunction(args *ParsedParams) (Function, error) {
	lenArg, err := args.FieldOptionalInt64("length")
	if err != nil {
		return nil, err
	}
	alphabetArg, err := args.FieldOptionalString("alphabet")
	if err != nil {
		return nil, err
	}
	if alphabetArg != nil && lenArg == nil {
		return nil, errors.New("field length must be specified when an alphabet is specified")
	}
	return ClosureFunction("function nanoid", func(ctx FunctionContext) (any, error) {
		if alphabetArg != nil {
			return gonanoid.Generate(*alphabetArg, int(*lenArg))
		}
		if lenArg != nil {
			return gonanoid.New(int(*lenArg))
		}
		return gonanoid.New()
	}, nil), nil
}

//------------------------------------------------------------------------------

var _ = registerSimpleFunction(
	NewFunctionSpec(
		FunctionCategoryGeneral, "ksuid",
		"Generates a K-Sortable Unique Identifier with built-in timestamp ordering. Use this for distributed unique IDs that sort chronologically and remain collision-resistant without coordination between generators.",
		NewExampleSpec("", `root.id = ksuid()`),
		NewExampleSpec("Create sortable event IDs for logging.",
			`root.event = {
  "id": ksuid(),
  "type": this.event_type,
  "data": this.payload
}`,
		),
	),
	func(_ FunctionContext) (any, error) {
		return ksuid.New().String(), nil
	},
)

//------------------------------------------------------------------------------

var _ = registerFunction(
	NewHiddenFunctionSpec("var").Param(ParamString("name", "The name of the target variable.")),
	func(args *ParsedParams) (Function, error) {
		name, err := args.FieldString("name")
		if err != nil {
			return nil, err
		}
		return NewVarFunction(name), nil
	},
)

// NewVarFunction creates a new variable function.
func NewVarFunction(name string) Function {
	return ClosureFunction("variable "+name, func(ctx FunctionContext) (any, error) {
		if ctx.Vars == nil {
			return nil, errors.New("variables were undefined")
		}
		if res, ok := ctx.Vars[name]; ok {
			return res, nil
		}
		return nil, fmt.Errorf("variable '%v' undefined", name)
	}, func(ctx TargetsContext) (TargetsContext, []TargetPath) {
		paths := []TargetPath{
			NewTargetPath(TargetVariable, name),
		}
		ctx = ctx.WithValues(paths)
		return ctx, paths
	})
}

//------------------------------------------------------------------------------

var _ = registerFunction(
	NewFunctionSpec(
		FunctionCategoryGeneral, "bytes",
		"Creates a zero-initialized byte array of specified length. Use this to allocate fixed-size byte buffers for binary data manipulation or to generate padding.",
		NewExampleSpec("",
			`root.data = bytes(5)`,
			`{"data":"AAAAAAAK"}`,
		),
		NewExampleSpec("Create a buffer for binary operations.",
			`root.header = bytes(16)
root.payload = content()`,
		),
	).Param(ParamInt64("length", "The size of the resulting byte array.")),
	func(args *ParsedParams) (Function, error) {
		length, err := args.FieldInt64("length")
		if err != nil {
			return nil, err
		}
		return ClosureFunction("function bytes", func(_ FunctionContext) (any, error) {
			return make([]byte, length), nil
		}, nil), nil
	},
)
