package eval

import (
	"fmt"
	"math"
	"time"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/syntax"
)

func (interp *Interpreter) evalBinaryOp(op syntax.TokenType, left, right any) any {
	// Timestamp subtraction: ts - ts = int64 nanoseconds.
	if op == syntax.MINUS {
		if lt, ok := left.(time.Time); ok {
			if rt, ok := right.(time.Time); ok {
				return lt.Sub(rt).Nanoseconds()
			}
			return NewError(fmt.Sprintf("cannot subtract %T from timestamp", right))
		}
	}
	// Timestamp comparison.
	if lt, ok := left.(time.Time); ok {
		if rt, ok := right.(time.Time); ok {
			switch op {
			case syntax.GT:
				return lt.After(rt)
			case syntax.GE:
				return !lt.Before(rt)
			case syntax.LT:
				return lt.Before(rt)
			case syntax.LE:
				return !lt.After(rt)
			case syntax.EQ:
				return lt.Equal(rt)
			case syntax.NE:
				return !lt.Equal(rt)
			default:
				return NewError(fmt.Sprintf("unsupported timestamp operation %s", op))
			}
		}
		if op == syntax.EQ || op == syntax.NE {
			// Cross-family: always false/true.
			return op == syntax.NE
		}
		return NewError(fmt.Sprintf("cannot compare timestamp with %T", right))
	}
	// Reject timestamp on right side for arithmetic.
	if _, ok := right.(time.Time); ok {
		if op == syntax.EQ || op == syntax.NE {
			return op == syntax.NE // cross-family
		}
		return NewError(fmt.Sprintf("cannot use %s with timestamp", op))
	}

	switch op {
	case syntax.PLUS:
		return evalAdd(left, right)
	case syntax.MINUS:
		return evalArith(left, right, "-")
	case syntax.STAR:
		return evalArith(left, right, "*")
	case syntax.SLASH:
		return evalDivide(left, right)
	case syntax.PERCENT:
		return evalModulo(left, right)
	case syntax.EQ:
		return valuesEqual(left, right)
	case syntax.NE:
		return !valuesEqual(left, right)
	case syntax.GT:
		return evalCompare(left, right, ">")
	case syntax.GE:
		return evalCompare(left, right, ">=")
	case syntax.LT:
		return evalCompare(left, right, "<")
	case syntax.LE:
		return evalCompare(left, right, "<=")
	default:
		return NewError(fmt.Sprintf("unknown binary operator %s", op))
	}
}

// valuesEqual implements Bloblang equality semantics.
func valuesEqual(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	// Numeric equality with promotion.
	if isNumeric(a) && isNumeric(b) {
		return numericEqual(a, b)
	}

	// Timestamp equality.
	if at, ok := a.(time.Time); ok {
		if bt, ok := b.(time.Time); ok {
			return at.Equal(bt)
		}
		return false // cross-family
	}

	// Bytes equality.
	if ab, ok := a.([]byte); ok {
		if bb, ok := b.([]byte); ok {
			if len(ab) != len(bb) {
				return false
			}
			for i := range ab {
				if ab[i] != bb[i] {
					return false
				}
			}
			return true
		}
		return false // cross-family
	}

	// Same type required for non-numeric.
	switch av := a.(type) {
	case string:
		bv, ok := b.(string)
		return ok && av == bv
	case bool:
		bv, ok := b.(bool)
		return ok && av == bv
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !valuesEqual(av[i], bv[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, v := range av {
			bval, exists := bv[k]
			if !exists || !valuesEqual(v, bval) {
				return false
			}
		}
		return true
	default:
		// Cross-family: always false.
		return false
	}
}

func isNumeric(v any) bool {
	switch v.(type) {
	case int32, int64, uint32, uint64, float32, float64:
		return true
	default:
		return false
	}
}

func numericEqual(a, b any) bool {
	// Same type: compare directly (no promotion, no precision loss).
	switch av := a.(type) {
	case int64:
		if bv, ok := b.(int64); ok {
			return av == bv
		}
	case int32:
		if bv, ok := b.(int32); ok {
			return av == bv
		}
	case uint32:
		if bv, ok := b.(uint32); ok {
			return av == bv
		}
	case uint64:
		if bv, ok := b.(uint64); ok {
			return av == bv
		}
	case float64:
		if bv, ok := b.(float64); ok {
			if math.IsNaN(av) || math.IsNaN(bv) {
				return false
			}
			return av == bv
		}
	case float32:
		if bv, ok := b.(float32); ok {
			if math.IsNaN(float64(av)) || math.IsNaN(float64(bv)) {
				return false
			}
			return av == bv
		}
	}

	// Different numeric types: use checked promotion to float64.
	af, aOk := checkedToFloat64(a)
	bf, bOk := checkedToFloat64(b)
	if !aOk || !bOk {
		return false // promotion failed — not equal
	}
	if math.IsNaN(af) || math.IsNaN(bf) {
		return false
	}
	return af == bf
}

func toFloat64(v any) float64 {
	switch n := v.(type) {
	case int64:
		return float64(n)
	case int32:
		return float64(n)
	case uint32:
		return float64(n)
	case uint64:
		return float64(n)
	case float64:
		return n
	case float32:
		return float64(n)
	default:
		return math.NaN()
	}
}

func evalAdd(left, right any) any {
	// String concatenation.
	if ls, ok := left.(string); ok {
		rs, ok := right.(string)
		if !ok {
			return NewError(fmt.Sprintf("cannot add string and %T", right))
		}
		return ls + rs
	}
	if _, ok := right.(string); ok {
		return NewError(fmt.Sprintf("cannot add %T and string", left))
	}
	// Bytes concatenation.
	if lb, ok := left.([]byte); ok {
		rb, ok := right.([]byte)
		if !ok {
			return NewError(fmt.Sprintf("cannot add bytes and %T", right))
		}
		result := make([]byte, len(lb)+len(rb))
		copy(result, lb)
		copy(result[len(lb):], rb)
		return result
	}
	// Numeric addition.
	return evalArith(left, right, "+")
}

func evalArith(left, right any, op string) any {
	if !isNumeric(left) || !isNumeric(right) {
		return NewError(fmt.Sprintf("cannot %s %T and %T", opVerb(op), left, right))
	}

	// Promote to common type.
	pl, pr, kind, promErr := promoteChecked(left, right)
	if promErr != "" {
		return NewError(promErr)
	}
	_ = kind

	switch kind {
	case promoteInt64:
		a, b := pl.(int64), pr.(int64)
		return checkedInt64Arith(a, b, op)
	case promoteInt32:
		a, b := pl.(int32), pr.(int32)
		return checkedInt32Arith(a, b, op)
	case promoteUint32:
		a, b := pl.(uint32), pr.(uint32)
		return checkedUint32Arith(a, b, op)
	case promoteUint64:
		a, b := pl.(uint64), pr.(uint64)
		return checkedUint64Arith(a, b, op)
	case promoteFloat64:
		a, b := pl.(float64), pr.(float64)
		return floatArith(a, b, op)
	case promoteFloat32:
		a, b := pl.(float32), pr.(float32)
		return float32Arith(a, b, op)
	default:
		return NewError("unexpected promotion result")
	}
}

func evalDivide(left, right any) any {
	if !isNumeric(left) || !isNumeric(right) {
		return NewError(fmt.Sprintf("cannot divide %T by %T", left, right))
	}

	// Division always produces float.
	// float32 / float32 → float32, all else → float64.
	_, isLF32 := left.(float32)
	_, isRF32 := right.(float32)
	if isLF32 && isRF32 {
		a, b := left.(float32), right.(float32)
		if b == 0 {
			return NewError("division by zero")
		}
		return a / b
	}

	af, bf := toFloat64(left), toFloat64(right)
	if bf == 0 {
		return NewError("division by zero")
	}
	return af / bf
}

func evalModulo(left, right any) any {
	if !isNumeric(left) || !isNumeric(right) {
		return NewError(fmt.Sprintf("cannot modulo %T by %T", left, right))
	}

	pl, pr, kind, promErr := promoteChecked(left, right)
	if promErr != "" {
		return NewError(promErr)
	}

	switch kind {
	case promoteInt64:
		a, b := pl.(int64), pr.(int64)
		if b == 0 {
			return NewError("modulo by zero")
		}
		return a % b
	case promoteInt32:
		a, b := pl.(int32), pr.(int32)
		if b == 0 {
			return NewError("modulo by zero")
		}
		return a % b
	case promoteUint32:
		a, b := pl.(uint32), pr.(uint32)
		if b == 0 {
			return NewError("modulo by zero")
		}
		return a % b
	case promoteUint64:
		a, b := pl.(uint64), pr.(uint64)
		if b == 0 {
			return NewError("modulo by zero")
		}
		return a % b
	case promoteFloat64:
		a, b := pl.(float64), pr.(float64)
		if b == 0 {
			return NewError("modulo by zero")
		}
		return math.Mod(a, b)
	case promoteFloat32:
		a, b := pl.(float32), pr.(float32)
		if b == 0 {
			return NewError("modulo by zero")
		}
		return float32(math.Mod(float64(a), float64(b)))
	default:
		return NewError("unexpected promotion result")
	}
}

func evalCompare(left, right any, op string) any {
	// Timestamp comparison.
	if lt, ok := left.(time.Time); ok {
		rt, ok := right.(time.Time)
		if !ok {
			return NewError(fmt.Sprintf("cannot compare timestamp and %T", right))
		}
		return timestampCompare(lt, rt, op)
	}

	if !isNumeric(left) && !isNumeric(right) {
		// String comparison.
		if ls, ok := left.(string); ok {
			rs, ok := right.(string)
			if !ok {
				return NewError(fmt.Sprintf("cannot compare string and %T", right))
			}
			return stringCompare(ls, rs, op)
		}
		return NewError(fmt.Sprintf("cannot compare %T and %T", left, right))
	}
	if !isNumeric(left) || !isNumeric(right) {
		return NewError(fmt.Sprintf("cannot compare %T and %T", left, right))
	}

	af, bf := toFloat64(left), toFloat64(right)
	switch op {
	case ">":
		return af > bf
	case ">=":
		return af >= bf
	case "<":
		return af < bf
	case "<=":
		return af <= bf
	default:
		return false
	}
}

func opVerb(op string) string {
	switch op {
	case "+":
		return "add"
	case "-":
		return "subtract"
	case "*":
		return "multiply"
	default:
		return "perform arithmetic on"
	}
}

func timestampCompare(a, b time.Time, op string) any {
	switch op {
	case ">":
		return a.After(b)
	case ">=":
		return !a.Before(b)
	case "<":
		return a.Before(b)
	case "<=":
		return !a.After(b)
	default:
		return false
	}
}

func stringCompare(a, b, op string) any {
	switch op {
	case ">":
		return a > b
	case ">=":
		return a >= b
	case "<":
		return a < b
	case "<=":
		return a <= b
	default:
		return false
	}
}

// -----------------------------------------------------------------------
// Numeric promotion
// -----------------------------------------------------------------------

type promoteKind int

const (
	promoteError promoteKind = iota
	promoteInt32
	promoteInt64
	promoteUint32
	promoteUint64
	promoteFloat32
	promoteFloat64
)

// promoteChecked promotes two values and returns a specific error message on failure.
func promoteChecked(a, b any) (any, any, promoteKind, string) {
	pa, pb, kind := promote(a, b)
	if kind == promoteError {
		// Determine specific error.
		ak, bk := numericKind(a), numericKind(b)
		if (ak == promoteUint64 || bk == promoteUint64) && !isFloatKind(ak) && !isFloatKind(bk) {
			return nil, nil, promoteError, "uint64 value exceeds int64 range"
		}
		return nil, nil, promoteError, "integer exceeds float64 exact range (magnitude > 2^53)"
	}
	return pa, pb, kind, ""
}

func promote(a, b any) (any, any, promoteKind) {
	ak, bk := numericKind(a), numericKind(b)

	if ak == bk {
		return a, b, ak
	}

	// Same signedness, different width: widen.
	// uint32 + uint64 → uint64.
	if (ak == promoteUint32 && bk == promoteUint64) || (ak == promoteUint64 && bk == promoteUint32) {
		return toU64(a), toU64(b), promoteUint64
	}

	// Any float involved → float64 (except float32+float32 which stays float32,
	// but that case is handled by ak == bk above).
	if isFloatKind(ak) || isFloatKind(bk) {
		af, aOk := checkedToFloat64(a)
		bf, bOk := checkedToFloat64(b)
		if !aOk || !bOk {
			return nil, nil, promoteError
		}
		return af, bf, promoteFloat64
	}

	// Both integers: widen to int64. Check uint64 overflow.
	ai := toI64(a)
	bi := toI64(b)
	if ai == nil || bi == nil {
		return nil, nil, promoteError
	}
	return ai, bi, promoteInt64
}

func toU64(v any) any {
	switch n := v.(type) {
	case uint32:
		return uint64(n)
	case uint64:
		return n
	default:
		return nil
	}
}

func isFloatKind(k promoteKind) bool {
	return k == promoteFloat32 || k == promoteFloat64
}

// checkedToFloat64 converts a numeric value to float64, returning false
// if the value is an integer with magnitude > 2^53 (can't be represented exactly).
func checkedToFloat64(v any) (float64, bool) {
	const maxSafeInt = 1 << 53 // 9007199254740992
	switch n := v.(type) {
	case int64:
		if n > maxSafeInt || n < -maxSafeInt {
			return 0, false
		}
		return float64(n), true
	case int32:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		if n > maxSafeInt {
			return 0, false
		}
		return float64(n), true
	case float64:
		return n, true
	case float32:
		return float64(n), true
	default:
		return 0, false
	}
}

func numericKind(v any) promoteKind {
	switch v.(type) {
	case int32:
		return promoteInt32
	case int64:
		return promoteInt64
	case uint32:
		return promoteUint32
	case uint64:
		return promoteUint64
	case float32:
		return promoteFloat32
	case float64:
		return promoteFloat64
	default:
		return promoteError
	}
}

func toI64(v any) any {
	switch n := v.(type) {
	case int32:
		return int64(n)
	case int64:
		return n
	case uint32:
		return int64(n)
	case uint64:
		if n > math.MaxInt64 {
			return nil // caller should check
		}
		return int64(n)
	default:
		return nil
	}
}

// -----------------------------------------------------------------------
// Checked integer arithmetic
// -----------------------------------------------------------------------

func checkedInt64Arith(a, b int64, op string) any {
	switch op {
	case "+":
		if (b > 0 && a > math.MaxInt64-b) || (b < 0 && a < math.MinInt64-b) {
			return NewError("int64 overflow")
		}
		return a + b
	case "-":
		if (b < 0 && a > math.MaxInt64+b) || (b > 0 && a < math.MinInt64+b) {
			return NewError("int64 overflow")
		}
		return a - b
	case "*":
		if a != 0 && b != 0 {
			result := a * b
			if result/a != b {
				return NewError("int64 overflow")
			}
			return result
		}
		return a * b
	default:
		return NewError("unsupported int64 operation " + op)
	}
}

func checkedInt32Arith(a, b int32, op string) any {
	// Promote to int64, check, then narrow.
	result := checkedInt64Arith(int64(a), int64(b), op)
	if IsError(result) {
		return result
	}
	r := result.(int64)
	if r > math.MaxInt32 || r < math.MinInt32 {
		return NewError("int32 overflow")
	}
	return int32(r)
}

func checkedUint32Arith(a, b uint32, op string) any {
	switch op {
	case "+":
		if a > math.MaxUint32-b {
			return NewError("uint32 overflow")
		}
		return a + b
	case "-":
		if a < b {
			return NewError("uint32 overflow")
		}
		return a - b
	case "*":
		if a != 0 && b != 0 {
			result := a * b
			if result/a != b {
				return NewError("uint32 overflow")
			}
			return result
		}
		return a * b
	default:
		return NewError("unsupported uint32 operation " + op)
	}
}

func checkedUint64Arith(a, b uint64, op string) any {
	switch op {
	case "+":
		if a > math.MaxUint64-b {
			return NewError("uint64 overflow")
		}
		return a + b
	case "-":
		if a < b {
			return NewError("uint64 overflow")
		}
		return a - b
	case "*":
		if a != 0 && b != 0 {
			result := a * b
			if result/a != b {
				return NewError("uint64 overflow")
			}
			return result
		}
		return a * b
	default:
		return NewError("unsupported uint64 operation " + op)
	}
}

func floatArith(a, b float64, op string) any {
	switch op {
	case "+":
		return a + b
	case "-":
		return a - b
	case "*":
		return a * b
	default:
		return NewError("unsupported float64 operation " + op)
	}
}

func float32Arith(a, b float32, op string) any {
	switch op {
	case "+":
		return a + b
	case "-":
		return a - b
	case "*":
		return a * b
	default:
		return NewError("unsupported float32 operation " + op)
	}
}
