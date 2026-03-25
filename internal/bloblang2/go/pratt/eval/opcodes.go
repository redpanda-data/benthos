package eval

// MethodOpcode is a compile-time integer ID for a stdlib method, assigned
// dynamically at init time. Opcode 0 is reserved (unused).
type MethodOpcode = uint16

// FunctionOpcode is a compile-time integer ID for a stdlib function, assigned
// dynamically at init time. Opcode 0 is reserved (unused).
type FunctionOpcode = uint16

// Opcode tables and name-to-opcode mappings. Built once at init time by
// initSharedStdlib and shared read-only across all interpreters.
var (
	methodTable   []MethodSpec   // indexed by MethodOpcode
	functionTable []FunctionSpec // indexed by FunctionOpcode

	methodNameToOpcode   map[string]MethodOpcode
	functionNameToOpcode map[string]FunctionOpcode

	// lambdaOpcodeBase is the first opcode assigned to lambda methods.
	// Lambda opcodes are lambdaOpcodeBase, lambdaOpcodeBase+1, ...
	// At runtime, interp.lambdaTable[opcode - lambdaOpcodeBase] resolves them.
	lambdaOpcodeBase MethodOpcode

	// lambdaOpcodeOffsets maps lambda method names to their offset from
	// lambdaOpcodeBase. Used during RegisterLambdaMethods to populate the
	// per-interpreter lambdaTable.
	lambdaOpcodeOffsets map[string]uint16
)

// nextMethodOpcode and nextFunctionOpcode are used during init to assign
// sequential opcodes. They start at 1 (0 is reserved).
var (
	nextMethodOpcode   MethodOpcode   = 1
	nextFunctionOpcode FunctionOpcode = 1
)

// registerMethodOpcode assigns an opcode to a static method during init.
func registerMethodOpcode(name string, spec MethodSpec) {
	opcode := nextMethodOpcode
	nextMethodOpcode++

	methodNameToOpcode[name] = opcode

	// Grow table if needed.
	for int(opcode) >= len(methodTable) {
		methodTable = append(methodTable, MethodSpec{})
	}
	methodTable[opcode] = spec
}

// registerFunctionOpcode assigns an opcode to a stdlib function during init.
func registerFunctionOpcode(name string, spec FunctionSpec) {
	opcode := nextFunctionOpcode
	nextFunctionOpcode++

	functionNameToOpcode[name] = opcode

	// Grow table if needed.
	for int(opcode) >= len(functionTable) {
		functionTable = append(functionTable, FunctionSpec{})
	}
	functionTable[opcode] = spec
}

// registerLambdaMethodOpcode assigns an opcode to a lambda method during init.
// Lambda opcodes start at lambdaOpcodeBase and are stored in a separate
// per-interpreter slice at runtime.
func registerLambdaMethodOpcode(name string) {
	offset := uint16(len(lambdaOpcodeOffsets))
	lambdaOpcodeOffsets[name] = offset
	methodNameToOpcode[name] = lambdaOpcodeBase + MethodOpcode(offset)
}

// StdlibOpcodes returns the name-to-opcode mappings for methods and functions.
// Used by the resolver to annotate AST nodes at compile time. Returns nil-safe
// maps (never nil).
func StdlibOpcodes() (methods map[string]uint16, functions map[string]uint16) {
	return methodNameToOpcode, functionNameToOpcode
}
