package eval

import (
	"bytes"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/syntax"
)

// RegisterLambdaMethods registers methods that take lambda arguments and
// need access to the interpreter for evaluation.
func (interp *Interpreter) RegisterLambdaMethods() {
	interp.lambdaMethods = map[string]lambdaMethodFunc{
		"filter":         interp.methodFilter,
		"map":            interp.methodMap,
		"sort":           interp.methodSort,
		"sort_by":        interp.methodSortBy,
		"any":            interp.methodAny,
		"all":            interp.methodAll,
		"find":           interp.methodFind,
		"fold":           interp.methodFold,
		"unique":         interp.methodUnique,
		"without_index":  interp.methodWithoutIndex,
		"index_of":       interp.methodIndexOf,
		"slice":          interp.methodSlice,
		"map_values":     interp.methodMapValues,
		"map_keys":       interp.methodMapKeys,
		"map_entries":    interp.methodMapEntries,
		"filter_entries": interp.methodFilterEntries,
	}
}

type lambdaMethodFunc func(receiver any, args []syntax.CallArg) any

func (interp *Interpreter) methodFilter(receiver any, args []syntax.CallArg) any {
	arr, ok := receiver.([]any)
	if !ok {
		return NewError(fmt.Sprintf("filter() requires array, got %T", receiver))
	}
	lambda := interp.extractLambdaOrMapRef(args)
	if lambda == nil {
		return NewError("filter() requires a lambda argument")
	}
	var result []any
	for _, elem := range arr {
		val := interp.callLambda(lambda, []any{elem})
		if IsError(val) {
			return val
		}
		b, ok := val.(bool)
		if !ok {
			return NewError(fmt.Sprintf("filter() lambda must return bool, got %T", val))
		}
		if b {
			result = append(result, elem)
		}
	}
	if result == nil {
		result = []any{}
	}
	return result
}

func (interp *Interpreter) methodMap(receiver any, args []syntax.CallArg) any {
	arr, ok := receiver.([]any)
	if !ok {
		return NewError(fmt.Sprintf("map() requires array, got %T", receiver))
	}
	lambda := interp.extractLambdaOrMapRef(args)
	if lambda == nil {
		return NewError("map() requires a lambda argument")
	}
	var result []any
	for _, elem := range arr {
		val := interp.callLambda(lambda, []any{elem})
		if IsError(val) {
			return val
		}
		if IsVoid(val) {
			return NewError("map() lambda returned void (must return a value for every element)")
		}
		if IsDeleted(val) {
			continue
		}
		result = append(result, val)
	}
	if result == nil {
		result = []any{}
	}
	return result
}

func (interp *Interpreter) methodSort(receiver any, args []syntax.CallArg) any {
	arr, ok := receiver.([]any)
	if !ok {
		return NewError(fmt.Sprintf("sort() requires array, got %T", receiver))
	}
	if len(arr) == 0 {
		return []any{}
	}

	sorted := make([]any, len(arr))
	copy(sorted, arr)

	var sortErr any
	sort.SliceStable(sorted, func(i, j int) bool {
		if sortErr != nil {
			return false
		}
		cmp := compareForSort(sorted[i], sorted[j])
		if IsError(cmp) {
			sortErr = cmp
			return false
		}
		return cmp.(int64) < 0
	})
	if sortErr != nil {
		return sortErr
	}
	return sorted
}

func (interp *Interpreter) methodSortBy(receiver any, args []syntax.CallArg) any {
	arr, ok := receiver.([]any)
	if !ok {
		return NewError(fmt.Sprintf("sort_by() requires array, got %T", receiver))
	}
	lambda := interp.extractLambdaOrMapRef(args)
	if lambda == nil {
		return NewError("sort_by() requires a lambda argument")
	}

	// Extract keys.
	keys := make([]any, len(arr))
	for i, elem := range arr {
		key := interp.callLambda(lambda, []any{elem})
		if IsError(key) {
			return key
		}
		keys[i] = key
	}

	indices := make([]int, len(arr))
	for i := range indices {
		indices[i] = i
	}

	var sortErr any
	sort.SliceStable(indices, func(i, j int) bool {
		if sortErr != nil {
			return false
		}
		cmp := compareForSort(keys[indices[i]], keys[indices[j]])
		if IsError(cmp) {
			sortErr = cmp
			return false
		}
		return cmp.(int64) < 0
	})
	if sortErr != nil {
		return sortErr
	}

	result := make([]any, len(arr))
	for i, idx := range indices {
		result[i] = arr[idx]
	}
	return result
}

func (interp *Interpreter) methodAny(receiver any, args []syntax.CallArg) any {
	arr, ok := receiver.([]any)
	if !ok {
		return NewError(fmt.Sprintf("any() requires array, got %T", receiver))
	}
	lambda := interp.extractLambdaOrMapRef(args)
	if lambda == nil {
		return NewError("any() requires a lambda argument")
	}
	for _, elem := range arr {
		val := interp.callLambda(lambda, []any{elem})
		if IsError(val) {
			return val
		}
		b, ok := val.(bool)
		if !ok {
			return NewError(fmt.Sprintf("any() lambda must return bool, got %T", val))
		}
		if b {
			return true // short-circuit
		}
	}
	return false
}

func (interp *Interpreter) methodAll(receiver any, args []syntax.CallArg) any {
	arr, ok := receiver.([]any)
	if !ok {
		return NewError(fmt.Sprintf("all() requires array, got %T", receiver))
	}
	lambda := interp.extractLambdaOrMapRef(args)
	if lambda == nil {
		return NewError("all() requires a lambda argument")
	}
	for _, elem := range arr {
		val := interp.callLambda(lambda, []any{elem})
		if IsError(val) {
			return val
		}
		b, ok := val.(bool)
		if !ok {
			return NewError(fmt.Sprintf("all() lambda must return bool, got %T", val))
		}
		if !b {
			return false // short-circuit
		}
	}
	return true
}

func (interp *Interpreter) methodFind(receiver any, args []syntax.CallArg) any {
	arr, ok := receiver.([]any)
	if !ok {
		return NewError(fmt.Sprintf("find() requires array, got %T", receiver))
	}
	lambda := interp.extractLambdaOrMapRef(args)
	if lambda == nil {
		return NewError("find() requires a lambda argument")
	}
	for _, elem := range arr {
		val := interp.callLambda(lambda, []any{elem})
		if IsError(val) {
			return val
		}
		b, ok := val.(bool)
		if !ok {
			return NewError(fmt.Sprintf("find() lambda must return bool, got %T", val))
		}
		if b {
			return elem // short-circuit
		}
	}
	return Void
}

func (interp *Interpreter) methodFold(receiver any, args []syntax.CallArg) any {
	arr, ok := receiver.([]any)
	if !ok {
		return NewError(fmt.Sprintf("fold() requires array, got %T", receiver))
	}
	if len(args) != 2 {
		return NewError("fold() requires initial value and lambda arguments")
	}
	initial := interp.evalExpr(args[0].Value)
	if IsError(initial) {
		return initial
	}
	lambda, ok := args[1].Value.(*syntax.LambdaExpr)
	if !ok {
		return NewError("fold() second argument must be a lambda")
	}

	tally := initial
	for _, elem := range arr {
		tally = interp.callLambda(lambda, []any{tally, elem})
		if IsError(tally) {
			return tally
		}
		if IsVoid(tally) {
			return NewError("fold() lambda returned void")
		}
	}
	return tally
}

func (interp *Interpreter) methodUnique(receiver any, args []syntax.CallArg) any {
	arr, ok := receiver.([]any)
	if !ok {
		return NewError(fmt.Sprintf("unique() requires array, got %T", receiver))
	}

	var keyFn *syntax.LambdaExpr
	if len(args) > 0 {
		keyFn = interp.extractLambdaOrMapRef(args)
	}

	var seenList []any
	seenNaN := false
	contains := func(key any) bool {
		// NaN values are considered equal for unique() per spec.
		if isNaN(key) {
			if seenNaN {
				return true
			}
			seenNaN = true
			return false
		}
		for _, s := range seenList {
			if valuesEqual(s, key) {
				return true
			}
		}
		return false
	}

	var result []any
	for _, elem := range arr {
		var key any
		if keyFn != nil {
			key = interp.callLambda(keyFn, []any{elem})
			if IsError(key) {
				return key
			}
		} else {
			key = elem
		}
		if !contains(key) {
			seenList = append(seenList, key)
			result = append(result, elem)
		}
	}
	if result == nil {
		result = []any{}
	}
	return result
}

func (interp *Interpreter) methodWithoutIndex(receiver any, args []syntax.CallArg) any {
	arr, ok := receiver.([]any)
	if !ok {
		return NewError(fmt.Sprintf("without_index() requires array, got %T", receiver))
	}
	if len(args) != 1 {
		return NewError("without_index() requires one argument")
	}
	idxVal := interp.evalExpr(args[0].Value)
	if IsError(idxVal) {
		return idxVal
	}
	idx, ok := toInt64(idxVal)
	if !ok {
		return NewError("without_index() argument must be integer")
	}
	if idx < 0 {
		idx += int64(len(arr))
	}
	if idx < 0 || idx >= int64(len(arr)) {
		return NewError("without_index(): index out of bounds")
	}
	result := make([]any, 0, len(arr)-1)
	result = append(result, arr[:idx]...)
	result = append(result, arr[idx+1:]...)
	return result
}

func (interp *Interpreter) methodIndexOf(receiver any, args []syntax.CallArg) any {
	if len(args) != 1 {
		return NewError("index_of() requires one argument")
	}
	target := interp.evalExpr(args[0].Value)
	if IsError(target) {
		return target
	}

	switch v := receiver.(type) {
	case string:
		s, ok := target.(string)
		if !ok {
			return NewError("string index_of() requires string argument")
		}
		idx := -1
		runes := []rune(v)
		targetRunes := []rune(s)
		for i := 0; i <= len(runes)-len(targetRunes); i++ {
			if string(runes[i:i+len(targetRunes)]) == s {
				idx = i
				break
			}
		}
		return int64(idx)
	case []any:
		for i, elem := range v {
			if valuesEqual(elem, target) {
				return int64(i)
			}
		}
		return int64(-1)
	case []byte:
		tb, ok := target.([]byte)
		if !ok {
			return NewError("bytes index_of() requires bytes argument")
		}
		return int64(bytes.Index(v, tb))
	default:
		return NewError(fmt.Sprintf("index_of() not supported on %T", receiver))
	}
}

func (interp *Interpreter) methodSlice(receiver any, args []syntax.CallArg) any {
	if len(args) < 1 || len(args) > 2 {
		return NewError("slice() requires 1 or 2 arguments")
	}
	lowVal := interp.evalExpr(args[0].Value)
	if IsError(lowVal) {
		return lowVal
	}
	low, ok := toInt64(lowVal)
	if !ok {
		return NewError("slice() low must be integer")
	}

	switch v := receiver.(type) {
	case string:
		runes := []rune(v)
		length := int64(len(runes))
		high := length
		if len(args) == 2 {
			hVal := interp.evalExpr(args[1].Value)
			if IsError(hVal) {
				return hVal
			}
			h, ok := toInt64(hVal)
			if !ok {
				return NewError("slice() high must be integer")
			}
			high = h
		}
		low, high = clampSlice(low, high, length)
		return string(runes[low:high])
	case []any:
		length := int64(len(v))
		high := length
		if len(args) == 2 {
			hVal := interp.evalExpr(args[1].Value)
			if IsError(hVal) {
				return hVal
			}
			h, ok := toInt64(hVal)
			if !ok {
				return NewError("slice() high must be integer")
			}
			high = h
		}
		low, high = clampSlice(low, high, length)
		result := make([]any, high-low)
		copy(result, v[low:high])
		return result
	case []byte:
		length := int64(len(v))
		high := length
		if len(args) == 2 {
			hVal := interp.evalExpr(args[1].Value)
			if IsError(hVal) {
				return hVal
			}
			h, ok := toInt64(hVal)
			if !ok {
				return NewError("slice() high must be integer")
			}
			high = h
		}
		low, high = clampSlice(low, high, length)
		result := make([]byte, high-low)
		copy(result, v[low:high])
		return result
	default:
		return NewError(fmt.Sprintf("slice() not supported on %T", receiver))
	}
}

func clampSlice(low, high, length int64) (int64, int64) {
	if low < 0 {
		low += length
	}
	if high < 0 {
		high += length
	}
	if low < 0 {
		low = 0
	}
	if high > length {
		high = length
	}
	if low > high {
		low = high
	}
	return low, high
}

func (interp *Interpreter) methodMapValues(receiver any, args []syntax.CallArg) any {
	obj, ok := receiver.(map[string]any)
	if !ok {
		return NewError(fmt.Sprintf("map_values() requires object, got %T", receiver))
	}
	lambda := interp.extractLambdaOrMapRef(args)
	if lambda == nil {
		return NewError("map_values() requires a lambda argument")
	}
	result := make(map[string]any, len(obj))
	for k, v := range obj {
		val := interp.callLambda(lambda, []any{v})
		if IsError(val) {
			return val
		}
		if IsVoid(val) {
			return NewError("map_values() lambda returned void")
		}
		if IsDeleted(val) {
			continue
		}
		result[k] = val
	}
	return result
}

func (interp *Interpreter) methodMapKeys(receiver any, args []syntax.CallArg) any {
	obj, ok := receiver.(map[string]any)
	if !ok {
		return NewError(fmt.Sprintf("map_keys() requires object, got %T", receiver))
	}
	lambda := interp.extractLambdaOrMapRef(args)
	if lambda == nil {
		return NewError("map_keys() requires a lambda argument")
	}
	result := make(map[string]any, len(obj))
	for k, v := range obj {
		newKey := interp.callLambda(lambda, []any{k})
		if IsError(newKey) {
			return newKey
		}
		if IsDeleted(newKey) {
			continue
		}
		s, ok := newKey.(string)
		if !ok {
			return NewError(fmt.Sprintf("map_keys() lambda must return string, got %T", newKey))
		}
		result[s] = v
	}
	return result
}

func (interp *Interpreter) methodMapEntries(receiver any, args []syntax.CallArg) any {
	obj, ok := receiver.(map[string]any)
	if !ok {
		return NewError(fmt.Sprintf("map_entries() requires object, got %T", receiver))
	}
	lambda := interp.extractLambdaOrMapRef(args)
	if lambda == nil {
		return NewError("map_entries() requires a lambda argument")
	}
	result := make(map[string]any, len(obj))
	for k, v := range obj {
		entry := interp.callLambda(lambda, []any{k, v})
		if IsError(entry) {
			return entry
		}
		if IsVoid(entry) {
			return NewError("map_entries() lambda returned void")
		}
		if IsDeleted(entry) {
			continue
		}
		entryMap, ok := entry.(map[string]any)
		if !ok {
			return NewError("map_entries() lambda must return {key, value} object")
		}
		key, ok := entryMap["key"].(string)
		if !ok {
			return NewError("map_entries() returned entry missing string 'key'")
		}
		val, exists := entryMap["value"]
		if !exists {
			return NewError("map_entries() returned entry missing 'value'")
		}
		result[key] = val
	}
	return result
}

func (interp *Interpreter) methodFilterEntries(receiver any, args []syntax.CallArg) any {
	obj, ok := receiver.(map[string]any)
	if !ok {
		return NewError(fmt.Sprintf("filter_entries() requires object, got %T", receiver))
	}
	lambda := interp.extractLambdaOrMapRef(args)
	if lambda == nil {
		return NewError("filter_entries() requires a lambda argument")
	}
	result := make(map[string]any, len(obj))
	for k, v := range obj {
		val := interp.callLambda(lambda, []any{k, v})
		if IsError(val) {
			return val
		}
		b, ok := val.(bool)
		if !ok {
			return NewError(fmt.Sprintf("filter_entries() lambda must return bool, got %T", val))
		}
		if b {
			result[k] = v
		}
	}
	return result
}

// extractLambdaOrMapRef gets the lambda expression from the first argument.
// If the argument is a bare identifier (map name), synthesizes a lambda
// that calls the map with a single parameter (Section 5.5).
func (interp *Interpreter) extractLambdaOrMapRef(args []syntax.CallArg) *syntax.LambdaExpr {
	if len(args) == 0 {
		return nil
	}

	// Direct lambda.
	if lambda, ok := args[0].Value.(*syntax.LambdaExpr); ok {
		return lambda
	}

	// Bare identifier → map name reference. Synthesize a lambda.
	if ident, ok := args[0].Value.(*syntax.IdentExpr); ok {
		if _, exists := interp.maps[ident.Name]; exists {
			return &syntax.LambdaExpr{
				TokenPos: ident.TokenPos,
				Params:   []syntax.Param{{Name: "__arg", Pos: ident.TokenPos}},
				Body: &syntax.ExprBody{
					Result: &syntax.CallExpr{
						TokenPos: ident.TokenPos,
						Name:     ident.Name,
						Args:     []syntax.CallArg{{Value: &syntax.IdentExpr{TokenPos: ident.TokenPos, Name: "__arg"}}},
					},
				},
			}
		}
		// Check namespaced references.
		if call, ok := args[0].Value.(*syntax.CallExpr); ok && call.Namespace != "" {
			// Already a call, shouldn't reach here.
			_ = call
		}
	}

	return nil
}

// compareForSort compares two values for sort ordering. Returns -1, 0, or 1.
func compareForSort(a, b any) any {
	// Handle NaN: sorts after everything.
	aNaN := isNaN(a)
	bNaN := isNaN(b)
	if aNaN && bNaN {
		return int64(0)
	}
	if aNaN {
		return int64(1)
	}
	if bNaN {
		return int64(-1)
	}

	// Numeric comparison.
	if isNumeric(a) && isNumeric(b) {
		af, bf := toFloat64(a), toFloat64(b)
		if af < bf {
			return int64(-1)
		}
		if af > bf {
			return int64(1)
		}
		return int64(0)
	}

	// String comparison.
	if as, ok := a.(string); ok {
		if bs, ok := b.(string); ok {
			if as < bs {
				return int64(-1)
			}
			if as > bs {
				return int64(1)
			}
			return int64(0)
		}
	}

	// Timestamp comparison.
	if at, ok := a.(time.Time); ok {
		if bt, ok := b.(time.Time); ok {
			if at.Before(bt) {
				return int64(-1)
			}
			if at.After(bt) {
				return int64(1)
			}
			return int64(0)
		}
	}

	return NewError(fmt.Sprintf("cannot sort: incompatible types %T and %T", a, b))
}

func isNaN(v any) bool {
	switch n := v.(type) {
	case float64:
		return math.IsNaN(n)
	case float32:
		return math.IsNaN(float64(n))
	default:
		return false
	}
}
