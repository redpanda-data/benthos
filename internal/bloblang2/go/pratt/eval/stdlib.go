package eval

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// RegisterStdlib registers all standard library functions and methods.
func (interp *Interpreter) RegisterStdlib() {
	interp.registerFunctions()
	interp.registerMethods()
}

// StdlibFunctionNames returns the names of all stdlib functions.
func StdlibFunctionNames() map[string]bool {
	return map[string]bool{
		"uuid_v4": true, "now": true, "random_int": true, "range": true,
		"timestamp": true, "second": true, "minute": true, "hour": true, "day": true,
		"throw": true, "deleted": true,
	}
}

// StdlibMethodNames returns the names of all stdlib methods.
func StdlibMethodNames() map[string]bool {
	return map[string]bool{
		// Type conversion.
		"string": true, "int32": true, "int64": true, "uint32": true, "uint64": true,
		"float32": true, "float64": true, "bool": true, "char": true, "bytes": true,
		// Introspection.
		"type": true,
		// Sequence.
		"length": true, "contains": true, "index_of": true, "slice": true, "reverse": true,
		// String.
		"uppercase": true, "lowercase": true, "trim": true, "trim_prefix": true, "trim_suffix": true,
		"has_prefix": true, "has_suffix": true, "split": true, "replace_all": true, "repeat": true,
		"re_match": true, "re_find_all": true, "re_replace_all": true,
		// Array.
		"filter": true, "map": true, "sort": true, "sort_by": true,
		"append": true, "concat": true, "flatten": true, "unique": true,
		"without_index": true, "enumerate": true, "any": true, "all": true,
		"find": true, "join": true, "sum": true, "min": true, "max": true,
		"fold": true, "collect": true,
		// Object.
		"iter": true, "keys": true, "values": true, "has_key": true,
		"merge": true, "without": true, "map_values": true, "map_keys": true,
		"map_entries": true, "filter_entries": true,
		// Numeric.
		"abs": true, "floor": true, "ceil": true, "round": true,
		// Timestamp.
		"ts_parse": true, "ts_format": true,
		"ts_unix": true, "ts_unix_milli": true, "ts_unix_micro": true, "ts_unix_nano": true,
		"ts_from_unix": true, "ts_from_unix_milli": true, "ts_from_unix_micro": true, "ts_from_unix_nano": true,
		"ts_add": true,
		// Error handling (intrinsics handled separately, but listed for validation).
		"catch": true, "or": true, "not_null": true,
		// Encoding.
		"parse_json": true, "format_json": true, "encode": true, "decode": true,
	}
}

func (interp *Interpreter) registerFunctions() {
	interp.RegisterFunction("deleted", func(_ []any) any { return Deleted })
	interp.RegisterFunction("throw", func(args []any) any {
		if len(args) != 1 {
			return NewError("throw() requires exactly one string argument")
		}
		msg, ok := args[0].(string)
		if !ok {
			return NewError(fmt.Sprintf("throw() requires a string argument, got %T", args[0]))
		}
		return NewError(msg)
	})
	interp.RegisterFunction("uuid_v4", func(_ []any) any {
		return uuid.New().String()
	})
	interp.RegisterFunction("now", func(_ []any) any {
		return time.Now().UTC()
	})
	interp.RegisterFunction("random_int", func(args []any) any {
		if len(args) != 2 {
			return NewError("random_int() requires min and max arguments")
		}
		minVal, ok1 := toInt64(args[0])
		maxVal, ok2 := toInt64(args[1])
		if !ok1 || !ok2 {
			return NewError("random_int() requires integer arguments")
		}
		if minVal > maxVal {
			return NewError("random_int(): min must be <= max")
		}
		return minVal + rand.Int64N(maxVal-minVal+1)
	})
	interp.RegisterFunction("range", func(args []any) any {
		if len(args) < 2 || len(args) > 3 {
			return NewError("range() requires 2 or 3 arguments")
		}
		start, ok1 := toInt64(args[0])
		stop, ok2 := toInt64(args[1])
		if !ok1 || !ok2 {
			return NewError("range() requires integer arguments")
		}
		var step int64
		if len(args) == 3 {
			s, ok := toInt64(args[2])
			if !ok {
				return NewError("range() step must be integer")
			}
			if s == 0 {
				return NewError("range() step cannot be zero")
			}
			if (start < stop && s < 0) || (start > stop && s > 0) {
				return NewError("range() step direction contradicts start/stop")
			}
			step = s
		} else {
			if start <= stop {
				step = 1
			} else {
				step = -1
			}
		}
		if start == stop {
			return []any{}
		}
		var result []any
		if step > 0 {
			for i := start; i < stop; i += step {
				result = append(result, i)
			}
		} else {
			for i := start; i > stop; i += step {
				result = append(result, i)
			}
		}
		return result
	})
	interp.RegisterFunction("second", func(_ []any) any { return int64(1_000_000_000) })
	interp.RegisterFunction("minute", func(_ []any) any { return int64(60_000_000_000) })
	interp.RegisterFunction("hour", func(_ []any) any { return int64(3_600_000_000_000) })
	interp.RegisterFunction("day", func(_ []any) any { return int64(86_400_000_000_000) })

	interp.RegisterFunction("timestamp", func(args []any) any {
		if len(args) < 3 {
			return NewError("timestamp() requires at least year, month, day")
		}
		year, ok1 := toInt64(args[0])
		month, ok2 := toInt64(args[1])
		day, ok3 := toInt64(args[2])
		if !ok1 || !ok2 || !ok3 {
			return NewError("timestamp() requires integer year, month, day")
		}
		var hour, minute, sec, nano int64
		tz := "UTC"
		if len(args) > 3 {
			hour, _ = toInt64(args[3])
		}
		if len(args) > 4 {
			minute, _ = toInt64(args[4])
		}
		if len(args) > 5 {
			sec, _ = toInt64(args[5])
		}
		if len(args) > 6 {
			nano, _ = toInt64(args[6])
		}
		if len(args) > 7 {
			if s, ok := args[7].(string); ok {
				tz = s
			}
		}
		// Validate ranges per spec.
		if month < 1 || month > 12 {
			return NewError(fmt.Sprintf("timestamp(): month %d out of range (1-12)", month))
		}
		if day < 1 || day > 31 {
			return NewError(fmt.Sprintf("timestamp(): day %d out of range (1-31)", day))
		}
		if hour < 0 || hour > 23 {
			return NewError(fmt.Sprintf("timestamp(): hour %d out of range (0-23)", hour))
		}
		if minute < 0 || minute > 59 {
			return NewError(fmt.Sprintf("timestamp(): minute %d out of range (0-59)", minute))
		}
		if sec < 0 || sec > 59 {
			return NewError(fmt.Sprintf("timestamp(): second %d out of range (0-59)", sec))
		}
		if nano < 0 || nano > 999999999 {
			return NewError(fmt.Sprintf("timestamp(): nano %d out of range (0-999999999)", nano))
		}
		loc, err := time.LoadLocation(tz)
		if err != nil {
			return NewError("timestamp(): unknown timezone " + tz)
		}
		return time.Date(int(year), time.Month(month), int(day), int(hour), int(minute), int(sec), int(nano), loc)
	})
}

func (interp *Interpreter) registerMethods() {
	m := func(fn MethodFunc) MethodSpec { return MethodSpec{Fn: fn} }

	// Type conversion and introspection.
	interp.RegisterMethod("type", MethodSpec{Fn: methodType, AcceptsNull: true})
	interp.RegisterMethod("string", MethodSpec{Fn: methodString, AcceptsNull: true})
	interp.RegisterMethod("int64", m(methodInt64))
	interp.RegisterMethod("int32", m(methodInt32))
	interp.RegisterMethod("uint32", m(methodUint32))
	interp.RegisterMethod("uint64", m(methodUint64))
	interp.RegisterMethod("float64", m(methodFloat64))
	interp.RegisterMethod("float32", m(methodFloat32))
	interp.RegisterMethod("bool", m(methodBool))
	interp.RegisterMethod("bytes", MethodSpec{Fn: methodBytes, AcceptsNull: true})
	interp.RegisterMethod("char", m(methodChar))

	// Sequence methods.
	interp.RegisterMethod("length", m(methodLength))
	interp.RegisterMethod("contains", m(methodContains))
	interp.RegisterMethod("reverse", m(methodReverse))

	// String methods.
	interp.RegisterMethod("uppercase", m(methodUppercase))
	interp.RegisterMethod("lowercase", m(methodLowercase))
	interp.RegisterMethod("trim", m(methodTrim))
	interp.RegisterMethod("trim_prefix", m(methodTrimPrefix))
	interp.RegisterMethod("trim_suffix", m(methodTrimSuffix))
	interp.RegisterMethod("has_prefix", m(methodHasPrefix))
	interp.RegisterMethod("has_suffix", m(methodHasSuffix))
	interp.RegisterMethod("split", m(methodSplit))
	interp.RegisterMethod("replace_all", m(methodReplaceAll))
	interp.RegisterMethod("repeat", m(methodRepeat))
	interp.RegisterMethod("re_match", m(methodReMatch))
	interp.RegisterMethod("re_find_all", m(methodReFindAll))
	interp.RegisterMethod("re_replace_all", m(methodReReplaceAll))

	// Numeric methods.
	interp.RegisterMethod("abs", m(methodAbs))
	interp.RegisterMethod("floor", m(methodFloor))
	interp.RegisterMethod("ceil", m(methodCeil))
	interp.RegisterMethod("round", m(methodRound))

	// Array methods.
	interp.RegisterMethod("append", m(methodAppend))
	interp.RegisterMethod("concat", m(methodConcat))
	interp.RegisterMethod("flatten", m(methodFlatten))
	interp.RegisterMethod("enumerate", m(methodEnumerate))
	interp.RegisterMethod("join", m(methodJoin))
	interp.RegisterMethod("sum", m(methodSum))
	interp.RegisterMethod("min", m(methodMin))
	interp.RegisterMethod("max", m(methodMax))

	// Object methods.
	interp.RegisterMethod("keys", m(methodKeys))
	interp.RegisterMethod("values", m(methodValues))
	interp.RegisterMethod("has_key", m(methodHasKey))
	interp.RegisterMethod("merge", m(methodMerge))
	interp.RegisterMethod("without", m(methodWithout))
	interp.RegisterMethod("iter", m(methodIter))
	interp.RegisterMethod("collect", m(methodCollect))

	// Timestamp methods.
	interp.RegisterMethod("ts_unix", m(methodTsUnix))
	interp.RegisterMethod("ts_unix_milli", m(methodTsUnixMilli))
	interp.RegisterMethod("ts_unix_micro", m(methodTsUnixMicro))
	interp.RegisterMethod("ts_unix_nano", m(methodTsUnixNano))
	interp.RegisterMethod("ts_from_unix", m(methodTsFromUnix))
	interp.RegisterMethod("ts_from_unix_milli", m(methodTsFromUnixMilli))
	interp.RegisterMethod("ts_from_unix_micro", m(methodTsFromUnixMicro))
	interp.RegisterMethod("ts_from_unix_nano", m(methodTsFromUnixNano))
	interp.RegisterMethod("ts_parse", MethodSpec{Fn: methodTsParse, Params: []MethodParam{
		{Name: "format", Default: defaultTimestampFormat, HasDefault: true},
	}})
	interp.RegisterMethod("ts_format", MethodSpec{Fn: methodTsFormat, Params: []MethodParam{
		{Name: "format", Default: defaultTimestampFormat, HasDefault: true},
	}})
	interp.RegisterMethod("ts_add", MethodSpec{Fn: methodTsAdd, Params: []MethodParam{
		{Name: "nanos"},
	}})

	// Encoding methods.
	interp.RegisterMethod("parse_json", m(methodParseJSON))
	interp.RegisterMethod("format_json", MethodSpec{Fn: methodFormatJSON, AcceptsNull: true, Params: []MethodParam{
		{Name: "indent", Default: "", HasDefault: true},
		{Name: "no_indent", Default: false, HasDefault: true},
		{Name: "escape_html", Default: true, HasDefault: true},
	}})
	interp.RegisterMethod("encode", MethodSpec{Fn: methodEncode, Params: []MethodParam{
		{Name: "scheme"},
	}})
	interp.RegisterMethod("decode", MethodSpec{Fn: methodDecode, Params: []MethodParam{
		{Name: "scheme"},
	}})

	// Error handling.
	interp.RegisterMethod("not_null", MethodSpec{Fn: methodNotNull, AcceptsNull: true, Params: []MethodParam{
		{Name: "message", Default: "unexpected null value", HasDefault: true},
	}})
}

// -----------------------------------------------------------------------
// Type introspection and conversion
// -----------------------------------------------------------------------

func methodType(receiver any, _ []any) any {
	if receiver == nil {
		return "null"
	}
	switch receiver.(type) {
	case string:
		return "string"
	case int32:
		return "int32"
	case int64:
		return "int64"
	case uint32:
		return "uint32"
	case uint64:
		return "uint64"
	case float32:
		return "float32"
	case float64:
		return "float64"
	case bool:
		return "bool"
	case []byte:
		return "bytes"
	case time.Time:
		return "timestamp"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return "unknown"
	}
}

func methodString(receiver any, _ []any) any {
	if receiver == nil {
		return "null"
	}
	switch v := receiver.(type) {
	case string:
		return v
	case int32:
		return strconv.FormatInt(int64(v), 10)
	case int64:
		return strconv.FormatInt(v, 10)
	case uint32:
		return strconv.FormatUint(uint64(v), 10)
	case uint64:
		return strconv.FormatUint(v, 10)
	case float32:
		return formatFloat(float64(v))
	case float64:
		return formatFloat(v)
	case bool:
		if v {
			return "true"
		}
		return "false"
	case time.Time:
		return formatTimestamp(v)
	case []byte:
		if !utf8.Valid(v) {
			return NewError("bytes are not valid UTF-8")
		}
		return string(v)
	case []any:
		if containsBytes(v) {
			return NewError("cannot convert array to string: contains bytes value (convert bytes explicitly before embedding in containers)")
		}
		b, err := json.Marshal(sortedJSON(v))
		if err != nil {
			return NewError("cannot convert array to string: " + err.Error())
		}
		return string(b)
	case map[string]any:
		if containsBytes(v) {
			return NewError("cannot convert object to string: contains bytes value (convert bytes explicitly before embedding in containers)")
		}
		b, err := json.Marshal(sortedJSON(v))
		if err != nil {
			return NewError("cannot convert object to string: " + err.Error())
		}
		return string(b)
	default:
		return NewError(fmt.Sprintf("cannot convert %T to string", receiver))
	}
}

// containsBytes recursively checks whether a value tree contains any []byte values.
func containsBytes(v any) bool {
	switch val := v.(type) {
	case []byte:
		return true
	case []any:
		for _, elem := range val {
			if containsBytes(elem) {
				return true
			}
		}
	case map[string]any:
		for _, elem := range val {
			if containsBytes(elem) {
				return true
			}
		}
	}
	return false
}

func formatFloat(f float64) string {
	if math.IsNaN(f) {
		return "NaN"
	}
	if math.IsInf(f, 1) {
		return "Infinity"
	}
	if math.IsInf(f, -1) {
		return "-Infinity"
	}
	if f == 0 && math.Signbit(f) {
		return "0.0" // negative zero
	}
	s := strconv.FormatFloat(f, 'g', -1, 64)
	// Ensure the string contains a decimal point or exponent.
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return s
}

func formatTimestamp(t time.Time) string {
	return strftimeFormat(t, defaultTimestampFormat)
}

func methodInt64(receiver any, _ []any) any {
	switch v := receiver.(type) {
	case int64:
		return v
	case int32:
		return int64(v)
	case uint32:
		return int64(v)
	case uint64:
		if v > math.MaxInt64 {
			return NewError("uint64 value exceeds int64 range")
		}
		return int64(v)
	case float64:
		return int64(v) // truncates toward zero
	case float32:
		return int64(v)
	case string:
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return NewError("cannot convert string to int64: " + err.Error())
		}
		return n
	case bool:
		return NewError("cannot convert bool to int64")
	default:
		return NewError(fmt.Sprintf("cannot convert %T to int64", receiver))
	}
}

func methodInt32(receiver any, _ []any) any {
	i64 := methodInt64(receiver, nil)
	if IsError(i64) {
		return i64
	}
	n := i64.(int64)
	if n > math.MaxInt32 || n < math.MinInt32 {
		return NewError("int32 overflow")
	}
	return int32(n)
}

func methodUint32(receiver any, _ []any) any {
	switch v := receiver.(type) {
	case uint32:
		return v
	case int64:
		if v < 0 || v > math.MaxUint32 {
			return NewError("uint32 overflow")
		}
		return uint32(v)
	case string:
		n, err := strconv.ParseUint(v, 10, 32)
		if err != nil {
			return NewError("cannot convert string to uint32: " + err.Error())
		}
		return uint32(n)
	default:
		i64 := methodInt64(receiver, nil)
		if IsError(i64) {
			return i64
		}
		n := i64.(int64)
		if n < 0 || n > math.MaxUint32 {
			return NewError("uint32 overflow")
		}
		return uint32(n)
	}
}

func methodUint64(receiver any, _ []any) any {
	switch v := receiver.(type) {
	case uint64:
		return v
	case int64:
		if v < 0 {
			return NewError("uint64 overflow: negative value")
		}
		return uint64(v)
	case string:
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			return NewError("uint64 overflow: " + err.Error())
		}
		return n
	default:
		i64 := methodInt64(receiver, nil)
		if IsError(i64) {
			return i64
		}
		n := i64.(int64)
		if n < 0 {
			return NewError("uint64 overflow: negative value")
		}
		return uint64(n)
	}
}

func methodFloat64(receiver any, _ []any) any {
	switch v := receiver.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int64:
		return float64(v)
	case int32:
		return float64(v)
	case uint32:
		return float64(v)
	case uint64:
		return float64(v)
	case string:
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return NewError("cannot convert string to float64: " + err.Error())
		}
		return f
	default:
		return NewError(fmt.Sprintf("cannot convert %T to float64", receiver))
	}
}

func methodFloat32(receiver any, _ []any) any {
	f64 := methodFloat64(receiver, nil)
	if IsError(f64) {
		return f64
	}
	return float32(f64.(float64))
}

func methodBool(receiver any, _ []any) any {
	switch v := receiver.(type) {
	case bool:
		return v
	case string:
		switch v {
		case "true":
			return true
		case "false":
			return false
		default:
			return NewError("cannot convert string " + strconv.Quote(v) + " to bool")
		}
	case int64:
		return v != 0
	case int32:
		return v != 0
	case uint32:
		return v != 0
	case uint64:
		return v != 0
	case float64:
		if math.IsNaN(v) {
			return NewError("NaN cannot be converted to bool")
		}
		return v != 0
	case float32:
		if math.IsNaN(float64(v)) {
			return NewError("NaN cannot be converted to bool")
		}
		return v != 0
	default:
		return NewError(fmt.Sprintf("cannot convert %T to bool", receiver))
	}
}

func methodBytes(receiver any, _ []any) any {
	switch v := receiver.(type) {
	case []byte:
		return v
	case string:
		return []byte(v)
	default:
		s := methodString(receiver, nil)
		if IsError(s) {
			return s
		}
		return []byte(s.(string))
	}
}

func methodChar(receiver any, _ []any) any {
	n, ok := toInt64(receiver)
	if !ok {
		return NewError(fmt.Sprintf("char() requires integer, got %T", receiver))
	}
	if n < 0 || n > 0x10FFFF {
		return NewError("codepoint out of valid Unicode range")
	}
	return string(rune(n))
}

func methodNotNull(receiver any, args []any) any {
	if receiver != nil {
		return receiver
	}
	msg := "unexpected null value"
	if len(args) > 0 {
		if s, ok := args[0].(string); ok {
			msg = s
		}
	}
	return NewError(msg)
}

// -----------------------------------------------------------------------
// Sequence methods
// -----------------------------------------------------------------------

func methodLength(receiver any, _ []any) any {
	switch v := receiver.(type) {
	case string:
		return int64(utf8.RuneCountInString(v))
	case []any:
		return int64(len(v))
	case []byte:
		return int64(len(v))
	case map[string]any:
		return int64(len(v))
	default:
		return NewError(fmt.Sprintf("length() not supported on %T", receiver))
	}
}

func methodContains(receiver any, args []any) any {
	if len(args) != 1 {
		return NewError("contains() requires exactly one argument")
	}
	switch v := receiver.(type) {
	case string:
		target, ok := args[0].(string)
		if !ok {
			return NewError("string contains() requires string argument")
		}
		return strings.Contains(v, target)
	case []any:
		for _, elem := range v {
			if valuesEqual(elem, args[0]) {
				return true
			}
		}
		return false
	case []byte:
		target, ok := args[0].([]byte)
		if !ok {
			return NewError("bytes contains() requires bytes argument")
		}
		return bytes.Contains(v, target)
	default:
		return NewError(fmt.Sprintf("contains() not supported on %T", receiver))
	}
}

func methodReverse(receiver any, _ []any) any {
	switch v := receiver.(type) {
	case string:
		runes := []rune(v)
		for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
			runes[i], runes[j] = runes[j], runes[i]
		}
		return string(runes)
	case []any:
		result := make([]any, len(v))
		for i, j := 0, len(v)-1; j >= 0; i, j = i+1, j-1 {
			result[i] = v[j]
		}
		return result
	case []byte:
		result := make([]byte, len(v))
		for i, j := 0, len(v)-1; j >= 0; i, j = i+1, j-1 {
			result[i] = v[j]
		}
		return result
	default:
		return NewError(fmt.Sprintf("reverse() not supported on %T", receiver))
	}
}

// -----------------------------------------------------------------------
// String methods
// -----------------------------------------------------------------------

func methodUppercase(receiver any, _ []any) any {
	s, ok := receiver.(string)
	if !ok {
		return NewError(fmt.Sprintf("uppercase() requires string, got %T", receiver))
	}
	return strings.ToUpper(s)
}

func methodLowercase(receiver any, _ []any) any {
	s, ok := receiver.(string)
	if !ok {
		return NewError(fmt.Sprintf("lowercase() requires string, got %T", receiver))
	}
	return strings.ToLower(s)
}

func methodTrim(receiver any, _ []any) any {
	s, ok := receiver.(string)
	if !ok {
		return NewError(fmt.Sprintf("trim() requires string, got %T", receiver))
	}
	return strings.TrimSpace(s)
}

func methodTrimPrefix(receiver any, args []any) any {
	s, ok := receiver.(string)
	if !ok {
		return NewError(fmt.Sprintf("trim_prefix() requires string, got %T", receiver))
	}
	if len(args) != 1 {
		return NewError("trim_prefix() requires one argument")
	}
	prefix, ok := args[0].(string)
	if !ok {
		return NewError("trim_prefix() argument must be string")
	}
	return strings.TrimPrefix(s, prefix)
}

func methodTrimSuffix(receiver any, args []any) any {
	s, ok := receiver.(string)
	if !ok {
		return NewError(fmt.Sprintf("trim_suffix() requires string, got %T", receiver))
	}
	if len(args) != 1 {
		return NewError("trim_suffix() requires one argument")
	}
	suffix, ok := args[0].(string)
	if !ok {
		return NewError("trim_suffix() argument must be string")
	}
	return strings.TrimSuffix(s, suffix)
}

func methodHasPrefix(receiver any, args []any) any {
	s, ok := receiver.(string)
	if !ok {
		return NewError(fmt.Sprintf("has_prefix() requires string, got %T", receiver))
	}
	if len(args) != 1 {
		return NewError("has_prefix() requires one argument")
	}
	prefix, ok := args[0].(string)
	if !ok {
		return NewError("has_prefix() argument must be string")
	}
	return strings.HasPrefix(s, prefix)
}

func methodHasSuffix(receiver any, args []any) any {
	s, ok := receiver.(string)
	if !ok {
		return NewError(fmt.Sprintf("has_suffix() requires string, got %T", receiver))
	}
	if len(args) != 1 {
		return NewError("has_suffix() requires one argument")
	}
	suffix, ok := args[0].(string)
	if !ok {
		return NewError("has_suffix() argument must be string")
	}
	return strings.HasSuffix(s, suffix)
}

func methodSplit(receiver any, args []any) any {
	s, ok := receiver.(string)
	if !ok {
		return NewError(fmt.Sprintf("split() requires string, got %T", receiver))
	}
	if len(args) != 1 {
		return NewError("split() requires one argument")
	}
	delim, ok := args[0].(string)
	if !ok {
		return NewError("split() argument must be string")
	}
	if delim == "" {
		if s == "" {
			return []any{}
		}
		// Split by codepoint.
		runes := []rune(s)
		result := make([]any, len(runes))
		for i, r := range runes {
			result[i] = string(r)
		}
		return result
	}
	parts := strings.Split(s, delim)
	result := make([]any, len(parts))
	for i, p := range parts {
		result[i] = p
	}
	return result
}

func methodReplaceAll(receiver any, args []any) any {
	s, ok := receiver.(string)
	if !ok {
		return NewError(fmt.Sprintf("replace_all() requires string, got %T", receiver))
	}
	if len(args) != 2 {
		return NewError("replace_all() requires old and new arguments")
	}
	old, ok1 := args[0].(string)
	new_, ok2 := args[1].(string)
	if !ok1 || !ok2 {
		return NewError("replace_all() arguments must be strings")
	}
	return strings.ReplaceAll(s, old, new_)
}

func methodRepeat(receiver any, args []any) any {
	s, ok := receiver.(string)
	if !ok {
		return NewError(fmt.Sprintf("repeat() requires string, got %T", receiver))
	}
	if len(args) != 1 {
		return NewError("repeat() requires one argument")
	}
	count, ok := toInt64(args[0])
	if !ok {
		return NewError("repeat() argument must be integer")
	}
	if count < 0 {
		return NewError("repeat() count must be non-negative")
	}
	return strings.Repeat(s, int(count))
}

func methodReMatch(receiver any, args []any) any {
	s, ok := receiver.(string)
	if !ok {
		return NewError(fmt.Sprintf("re_match() requires string, got %T", receiver))
	}
	if len(args) != 1 {
		return NewError("re_match() requires one argument")
	}
	pattern, ok := args[0].(string)
	if !ok {
		return NewError("re_match() argument must be string")
	}
	matched, err := regexp.MatchString(pattern, s)
	if err != nil {
		return NewError("re_match() invalid pattern: " + err.Error())
	}
	return matched
}

func methodReFindAll(receiver any, args []any) any {
	s, ok := receiver.(string)
	if !ok {
		return NewError(fmt.Sprintf("re_find_all() requires string, got %T", receiver))
	}
	if len(args) != 1 {
		return NewError("re_find_all() requires one argument")
	}
	pattern, ok := args[0].(string)
	if !ok {
		return NewError("re_find_all() argument must be string")
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return NewError("re_find_all() invalid pattern: " + err.Error())
	}
	matches := re.FindAllString(s, -1)
	result := make([]any, len(matches))
	for i, m := range matches {
		result[i] = m
	}
	return result
}

func methodReReplaceAll(receiver any, args []any) any {
	s, ok := receiver.(string)
	if !ok {
		return NewError(fmt.Sprintf("re_replace_all() requires string, got %T", receiver))
	}
	if len(args) != 2 {
		return NewError("re_replace_all() requires pattern and replacement arguments")
	}
	pattern, ok1 := args[0].(string)
	replacement, ok2 := args[1].(string)
	if !ok1 || !ok2 {
		return NewError("re_replace_all() arguments must be strings")
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return NewError("re_replace_all() invalid pattern: " + err.Error())
	}
	return re.ReplaceAllString(s, replacement)
}

// -----------------------------------------------------------------------
// Numeric methods
// -----------------------------------------------------------------------

func methodAbs(receiver any, _ []any) any {
	switch v := receiver.(type) {
	case int64:
		if v == math.MinInt64 {
			return NewError("int64 overflow in abs()")
		}
		if v < 0 {
			return -v
		}
		return v
	case int32:
		if v == math.MinInt32 {
			return NewError("int32 overflow in abs()")
		}
		if v < 0 {
			return -v
		}
		return v
	case float64:
		return math.Abs(v)
	case float32:
		return float32(math.Abs(float64(v)))
	case uint32:
		return v
	case uint64:
		return v
	default:
		return NewError(fmt.Sprintf("abs() requires numeric, got %T", receiver))
	}
}

func methodFloor(receiver any, _ []any) any {
	switch v := receiver.(type) {
	case float64:
		return math.Floor(v)
	case float32:
		return float32(math.Floor(float64(v)))
	default:
		return NewError(fmt.Sprintf("floor() requires float, got %T", receiver))
	}
}

func methodCeil(receiver any, _ []any) any {
	switch v := receiver.(type) {
	case float64:
		return math.Ceil(v)
	case float32:
		return float32(math.Ceil(float64(v)))
	default:
		return NewError(fmt.Sprintf("ceil() requires float, got %T", receiver))
	}
}

func methodRound(receiver any, args []any) any {
	var n int64
	if len(args) > 0 {
		var ok bool
		n, ok = toInt64(args[0])
		if !ok {
			return NewError("round() argument must be integer")
		}
	}

	switch v := receiver.(type) {
	case float64:
		return roundFloat(v, n)
	case float32:
		return float32(roundFloat(float64(v), n))
	default:
		return NewError(fmt.Sprintf("round() requires float, got %T", receiver))
	}
}

func roundFloat(f float64, decimals int64) float64 {
	shift := math.Pow(10, float64(decimals))
	return math.RoundToEven(f*shift) / shift
}

// -----------------------------------------------------------------------
// Array methods
// -----------------------------------------------------------------------

func methodAppend(receiver any, args []any) any {
	arr, ok := receiver.([]any)
	if !ok {
		return NewError(fmt.Sprintf("append() requires array, got %T", receiver))
	}
	if len(args) != 1 {
		return NewError("append() requires one argument")
	}
	result := make([]any, len(arr)+1)
	copy(result, arr)
	result[len(arr)] = args[0]
	return result
}

func methodConcat(receiver any, args []any) any {
	arr, ok := receiver.([]any)
	if !ok {
		return NewError(fmt.Sprintf("concat() requires array, got %T", receiver))
	}
	if len(args) != 1 {
		return NewError("concat() requires one argument")
	}
	other, ok := args[0].([]any)
	if !ok {
		return NewError("concat() argument must be array")
	}
	result := make([]any, len(arr)+len(other))
	copy(result, arr)
	copy(result[len(arr):], other)
	return result
}

func methodFlatten(receiver any, _ []any) any {
	arr, ok := receiver.([]any)
	if !ok {
		return NewError(fmt.Sprintf("flatten() requires array, got %T", receiver))
	}
	var result []any
	for _, elem := range arr {
		if inner, ok := elem.([]any); ok {
			result = append(result, inner...)
		} else {
			result = append(result, elem)
		}
	}
	return result
}

func methodEnumerate(receiver any, _ []any) any {
	arr, ok := receiver.([]any)
	if !ok {
		return NewError(fmt.Sprintf("enumerate() requires array, got %T", receiver))
	}
	result := make([]any, len(arr))
	for i, v := range arr {
		result[i] = map[string]any{"index": int64(i), "value": v}
	}
	return result
}

func methodJoin(receiver any, args []any) any {
	arr, ok := receiver.([]any)
	if !ok {
		return NewError(fmt.Sprintf("join() requires array, got %T", receiver))
	}
	if len(args) != 1 {
		return NewError("join() requires one argument")
	}
	delim, ok := args[0].(string)
	if !ok {
		return NewError("join() delimiter must be string")
	}
	parts := make([]string, len(arr))
	for i, elem := range arr {
		s, ok := elem.(string)
		if !ok {
			return NewError(fmt.Sprintf("join() requires all elements to be strings, element %d is %T", i, elem))
		}
		parts[i] = s
	}
	return strings.Join(parts, delim)
}

func methodSum(receiver any, _ []any) any {
	arr, ok := receiver.([]any)
	if !ok {
		return NewError(fmt.Sprintf("sum() requires array, got %T", receiver))
	}
	if len(arr) == 0 {
		return int64(0)
	}
	result := arr[0]
	for _, elem := range arr[1:] {
		result = evalAdd(result, elem)
		if IsError(result) {
			return result
		}
	}
	return result
}

func methodMin(receiver any, _ []any) any {
	arr, ok := receiver.([]any)
	if !ok {
		return NewError(fmt.Sprintf("min() requires array, got %T", receiver))
	}
	if len(arr) == 0 {
		return NewError("min() requires non-empty array")
	}
	result := arr[0]
	for _, elem := range arr[1:] {
		cmp := compareForSort(result, elem)
		if IsError(cmp) {
			return cmp
		}
		if cmp.(int64) > 0 {
			result = elem
		}
	}
	return result
}

func methodMax(receiver any, _ []any) any {
	arr, ok := receiver.([]any)
	if !ok {
		return NewError(fmt.Sprintf("max() requires array, got %T", receiver))
	}
	if len(arr) == 0 {
		return NewError("max() requires non-empty array")
	}
	result := arr[0]
	for _, elem := range arr[1:] {
		cmp := compareForSort(result, elem)
		if IsError(cmp) {
			return cmp
		}
		if cmp.(int64) < 0 {
			result = elem
		}
	}
	return result
}

// -----------------------------------------------------------------------
// Object methods
// -----------------------------------------------------------------------

func methodKeys(receiver any, _ []any) any {
	obj, ok := receiver.(map[string]any)
	if !ok {
		return NewError(fmt.Sprintf("keys() requires object, got %T", receiver))
	}
	result := make([]any, 0, len(obj))
	for k := range obj {
		result = append(result, k)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].(string) < result[j].(string)
	})
	return result
}

func methodValues(receiver any, _ []any) any {
	obj, ok := receiver.(map[string]any)
	if !ok {
		return NewError(fmt.Sprintf("values() requires object, got %T", receiver))
	}
	// Sort by keys for deterministic order.
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	result := make([]any, len(keys))
	for i, k := range keys {
		result[i] = obj[k]
	}
	return result
}

func methodHasKey(receiver any, args []any) any {
	obj, ok := receiver.(map[string]any)
	if !ok {
		return NewError(fmt.Sprintf("has_key() requires object, got %T", receiver))
	}
	if len(args) != 1 {
		return NewError("has_key() requires one argument")
	}
	key, ok := args[0].(string)
	if !ok {
		return NewError("has_key() argument must be string")
	}
	_, exists := obj[key]
	return exists
}

func methodMerge(receiver any, args []any) any {
	obj, ok := receiver.(map[string]any)
	if !ok {
		return NewError(fmt.Sprintf("merge() requires object, got %T", receiver))
	}
	if len(args) != 1 {
		return NewError("merge() requires one argument")
	}
	other, ok := args[0].(map[string]any)
	if !ok {
		return NewError("merge() argument must be object")
	}
	result := make(map[string]any, len(obj)+len(other))
	for k, v := range obj {
		result[k] = v
	}
	for k, v := range other {
		result[k] = v
	}
	return result
}

func methodWithout(receiver any, args []any) any {
	obj, ok := receiver.(map[string]any)
	if !ok {
		return NewError(fmt.Sprintf("without() requires object, got %T", receiver))
	}
	if len(args) != 1 {
		return NewError("without() requires one argument")
	}
	keys, ok := args[0].([]any)
	if !ok {
		return NewError("without() argument must be array of strings")
	}
	exclude := make(map[string]bool, len(keys))
	for _, k := range keys {
		if s, ok := k.(string); ok {
			exclude[s] = true
		}
	}
	result := make(map[string]any, len(obj))
	for k, v := range obj {
		if !exclude[k] {
			result[k] = v
		}
	}
	return result
}

func methodIter(receiver any, _ []any) any {
	obj, ok := receiver.(map[string]any)
	if !ok {
		return NewError(fmt.Sprintf("iter() requires object, got %T", receiver))
	}
	result := make([]any, 0, len(obj))
	for k, v := range obj {
		result = append(result, map[string]any{"key": k, "value": v})
	}
	return result
}

func methodCollect(receiver any, _ []any) any {
	arr, ok := receiver.([]any)
	if !ok {
		return NewError(fmt.Sprintf("collect() requires array, got %T", receiver))
	}
	result := make(map[string]any, len(arr))
	for _, elem := range arr {
		entry, ok := elem.(map[string]any)
		if !ok {
			return NewError("collect() requires array of {key, value} objects")
		}
		key, ok := entry["key"].(string)
		if !ok {
			return NewError("collect() entry missing string 'key' field")
		}
		val, ok := entry["value"]
		if !ok {
			return NewError("collect() entry missing 'value' field")
		}
		result[key] = val
	}
	return result
}

// -----------------------------------------------------------------------
// Timestamp methods
// -----------------------------------------------------------------------

func methodTsUnix(receiver any, _ []any) any {
	t, ok := receiver.(time.Time)
	if !ok {
		return NewError(fmt.Sprintf("ts_unix() requires timestamp, got %T", receiver))
	}
	return t.Unix()
}

func methodTsUnixMilli(receiver any, _ []any) any {
	t, ok := receiver.(time.Time)
	if !ok {
		return NewError(fmt.Sprintf("ts_unix_milli() requires timestamp, got %T", receiver))
	}
	return t.UnixMilli()
}

func methodTsUnixMicro(receiver any, _ []any) any {
	t, ok := receiver.(time.Time)
	if !ok {
		return NewError(fmt.Sprintf("ts_unix_micro() requires timestamp, got %T", receiver))
	}
	return t.UnixMicro()
}

func methodTsUnixNano(receiver any, _ []any) any {
	t, ok := receiver.(time.Time)
	if !ok {
		return NewError(fmt.Sprintf("ts_unix_nano() requires timestamp, got %T", receiver))
	}
	return t.UnixNano()
}

func methodTsFromUnix(receiver any, _ []any) any {
	f := toFloat64(receiver)
	sec := int64(f)
	nsec := int64((f - float64(sec)) * 1e9)
	return time.Unix(sec, nsec).UTC()
}

func methodTsFromUnixMilli(receiver any, _ []any) any {
	n, ok := toInt64(receiver)
	if !ok {
		return NewError(fmt.Sprintf("ts_from_unix_milli() requires integer, got %T", receiver))
	}
	return time.UnixMilli(n).UTC()
}

func methodTsFromUnixMicro(receiver any, _ []any) any {
	n, ok := toInt64(receiver)
	if !ok {
		return NewError(fmt.Sprintf("ts_from_unix_micro() requires integer, got %T", receiver))
	}
	return time.UnixMicro(n).UTC()
}

func methodTsFromUnixNano(receiver any, _ []any) any {
	n, ok := toInt64(receiver)
	if !ok {
		return NewError(fmt.Sprintf("ts_from_unix_nano() requires integer, got %T", receiver))
	}
	return time.Unix(0, n).UTC()
}

func methodTsAdd(receiver any, args []any) any {
	t, ok := receiver.(time.Time)
	if !ok {
		return NewError(fmt.Sprintf("ts_add() requires timestamp, got %T", receiver))
	}
	if len(args) != 1 {
		return NewError("ts_add() requires one argument (nanoseconds)")
	}
	nanos, ok := toInt64(args[0])
	if !ok {
		return NewError("ts_add() argument must be integer nanoseconds")
	}
	return t.Add(time.Duration(nanos))
}

const defaultTimestampFormat = "%Y-%m-%dT%H:%M:%S%f%z"

func methodTsParse(receiver any, args []any) any {
	s, ok := receiver.(string)
	if !ok {
		return NewError(fmt.Sprintf("ts_parse() requires string, got %T", receiver))
	}

	format := defaultTimestampFormat
	if len(args) > 0 {
		if f, ok := args[0].(string); ok {
			format = f
		}
	}

	t, err := strftimeParse(s, format)
	if err != nil {
		return NewError("ts_parse() failed: " + err.Error())
	}
	return t
}

func methodTsFormat(receiver any, args []any) any {
	t, ok := receiver.(time.Time)
	if !ok {
		return NewError(fmt.Sprintf("ts_format() requires timestamp, got %T", receiver))
	}

	format := defaultTimestampFormat
	if len(args) > 0 {
		if f, ok := args[0].(string); ok {
			format = f
		}
	}

	return strftimeFormat(t, format)
}

// -----------------------------------------------------------------------
// Encoding methods
// -----------------------------------------------------------------------

func methodParseJSON(receiver any, _ []any) any {
	var data []byte
	switch v := receiver.(type) {
	case string:
		data = []byte(v)
	case []byte:
		data = v
	default:
		return NewError(fmt.Sprintf("parse_json() requires string or bytes, got %T", receiver))
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.UseNumber()
	var result any
	if err := dec.Decode(&result); err != nil {
		return NewError("parse_json() failed: " + err.Error())
	}
	return normalizeJSONNumbers(result)
}

func normalizeJSONNumbers(v any) any {
	switch val := v.(type) {
	case json.Number:
		s := val.String()
		// Spec: numbers with decimal or exponent → float64, else → int64.
		if strings.ContainsAny(s, ".eE") {
			f, err := val.Float64()
			if err != nil {
				return NewError("parse_json(): invalid number " + s)
			}
			return f
		}
		n, err := val.Int64()
		if err != nil {
			// Exceeds int64 range → float64 (may lose precision).
			f, err := val.Float64()
			if err != nil {
				return NewError("parse_json(): invalid number " + s)
			}
			return f
		}
		return n
	case map[string]any:
		for k, v := range val {
			val[k] = normalizeJSONNumbers(v)
		}
		return val
	case []any:
		for i, v := range val {
			val[i] = normalizeJSONNumbers(v)
		}
		return val
	default:
		return v
	}
}

func methodFormatJSON(receiver any, args []any) any {
	// Args mapped by RegisterMethodWithParams: [indent, no_indent, escape_html]
	indent := ""
	escapeHTML := true

	if len(args) > 0 {
		if s, ok := args[0].(string); ok {
			indent = s
		}
	}
	if len(args) > 1 {
		if b, ok := args[1].(bool); ok && b {
			indent = "" // no_indent overrides indent
		}
	}
	if len(args) > 2 {
		if b, ok := args[2].(bool); ok {
			escapeHTML = b
		}
	}

	// Check for non-JSON-representable values.
	if err := checkJSONSerializable(receiver); err != "" {
		return NewError(err)
	}

	// Use json.Encoder for escape_html control.
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(escapeHTML)
	if indent != "" {
		enc.SetIndent("", indent)
	}
	if err := enc.Encode(sortedJSON(receiver)); err != nil {
		return NewError("format_json() failed: " + err.Error())
	}
	// Encoder adds a trailing newline — remove it.
	result := buf.String()
	if len(result) > 0 && result[len(result)-1] == '\n' {
		result = result[:len(result)-1]
	}
	return result
}

func checkJSONSerializable(v any) string {
	switch val := v.(type) {
	case float64:
		if math.IsNaN(val) {
			return "format_json(): NaN is not representable in JSON"
		}
		if math.IsInf(val, 0) {
			return "format_json(): Infinity is not representable in JSON"
		}
	case float32:
		f := float64(val)
		if math.IsNaN(f) {
			return "format_json(): NaN is not representable in JSON"
		}
		if math.IsInf(f, 0) {
			return "format_json(): Infinity is not representable in JSON"
		}
	case []byte:
		return "format_json(): bytes have no implicit JSON serialization"
	case map[string]any:
		for _, v := range val {
			if err := checkJSONSerializable(v); err != "" {
				return err
			}
		}
	case []any:
		for _, v := range val {
			if err := checkJSONSerializable(v); err != "" {
				return err
			}
		}
	}
	return ""
}

// sortedJSON returns a value suitable for json.Marshal with sorted object keys.
func sortedJSON(v any) any {
	switch val := v.(type) {
	case map[string]any:
		// json.Marshal sorts keys by default in Go, so this is fine.
		sorted := make(map[string]any, len(val))
		for k, v := range val {
			sorted[k] = sortedJSON(v)
		}
		return sorted
	case []any:
		result := make([]any, len(val))
		for i, v := range val {
			result[i] = sortedJSON(v)
		}
		return result
	default:
		return v
	}
}

func methodEncode(receiver any, args []any) any {
	if len(args) != 1 {
		return NewError("encode() requires one argument (scheme)")
	}
	scheme, ok := args[0].(string)
	if !ok {
		return NewError("encode() scheme must be string")
	}
	var data []byte
	switch v := receiver.(type) {
	case string:
		data = []byte(v)
	case []byte:
		data = v
	default:
		return NewError(fmt.Sprintf("encode() requires string or bytes, got %T", receiver))
	}
	switch scheme {
	case "base64":
		return base64.StdEncoding.EncodeToString(data)
	case "base64url":
		return base64.URLEncoding.EncodeToString(data)
	case "base64rawurl":
		return base64.RawURLEncoding.EncodeToString(data)
	case "hex":
		return hex.EncodeToString(data)
	default:
		return NewError("encode(): unknown scheme " + scheme)
	}
}

func methodDecode(receiver any, args []any) any {
	s, ok := receiver.(string)
	if !ok {
		return NewError(fmt.Sprintf("decode() requires string, got %T", receiver))
	}
	if len(args) != 1 {
		return NewError("decode() requires one argument (scheme)")
	}
	scheme, ok := args[0].(string)
	if !ok {
		return NewError("decode() scheme must be string")
	}
	switch scheme {
	case "base64":
		b, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return NewError("decode() base64 failed: " + err.Error())
		}
		return b
	case "base64url":
		b, err := base64.URLEncoding.DecodeString(s)
		if err != nil {
			return NewError("decode() base64url failed: " + err.Error())
		}
		return b
	case "base64rawurl":
		b, err := base64.RawURLEncoding.DecodeString(s)
		if err != nil {
			return NewError("decode() base64rawurl failed: " + err.Error())
		}
		return b
	case "hex":
		b, err := hex.DecodeString(s)
		if err != nil {
			return NewError("decode() hex failed: " + err.Error())
		}
		return b
	default:
		return NewError("decode(): unknown scheme " + scheme)
	}
}
