package eval

// Special sentinel values used internally by the interpreter.
// These are distinct from normal Bloblang values (which use native Go types).

type (
	voidVal    struct{}
	deletedVal struct{}
	errorVal   struct{ message string }
)

// Void is the singleton void value. It represents the absence of a value,
// produced by if-without-else when the condition is false, or by
// match-without-wildcard when no case matches.
var Void = voidVal{}

// Deleted is the singleton deletion marker. When assigned to a field,
// it removes the field. When assigned to root output, it drops the message.
var Deleted = deletedVal{}

// NewError creates a runtime error value that propagates through
// postfix chains until caught by .catch().
func NewError(msg string) errorVal {
	return errorVal{message: msg}
}

// IsVoid reports whether v is the void sentinel.
func IsVoid(v any) bool {
	_, ok := v.(voidVal)
	return ok
}

// IsDeleted reports whether v is the deletion sentinel.
func IsDeleted(v any) bool {
	_, ok := v.(deletedVal)
	return ok
}

// IsError reports whether v is a runtime error value.
func IsError(v any) bool {
	_, ok := v.(errorVal)
	return ok
}

// ErrorMessage returns the error message if v is an errorVal, or empty string.
func ErrorMessage(v any) string {
	if e, ok := v.(errorVal); ok {
		return e.message
	}
	return ""
}
