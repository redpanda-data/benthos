package bloblspec

// Interpreter compiles and executes Bloblang V2 mappings.
type Interpreter interface {
	// Compile parses a mapping string. files provides a virtual filesystem
	// for import resolution (filename -> content). If compilation fails,
	// return a *CompileError.
	Compile(mapping string, files map[string]string) (Mapping, error)
}

// Mapping is a compiled Bloblang mapping ready for execution.
type Mapping interface {
	// Exec runs the mapping against the given input document and metadata.
	// Returns the output document, output metadata, whether the message was
	// deleted (output = deleted()), and any runtime error.
	//
	// Runtime errors must NOT be wrapped as *CompileError — the test runner
	// uses type assertion to distinguish compile errors from runtime errors.
	//
	// Values passed as input and returned as output use native Go types:
	//
	//   Bloblang type | Go type
	//   --------------|--------
	//   string        | string
	//   int32         | int32
	//   int64         | int64
	//   uint32        | uint32
	//   uint64        | uint64
	//   float32       | float32
	//   float64       | float64
	//   bool          | bool
	//   null          | nil
	//   bytes         | []byte
	//   timestamp     | time.Time
	//   array         | []any
	//   object        | map[string]any
	//
	// Metadata is always an object (map[string]any). When the mapping does
	// not modify metadata, return an empty map.
	Exec(input any, metadata map[string]any) (output any, outputMeta map[string]any, deleted bool, err error)
}

// CompileError indicates a compilation failure. The test runner uses
// errors.As to distinguish compile errors from runtime errors.
type CompileError struct {
	Message string
}

func (e *CompileError) Error() string { return e.Message }
