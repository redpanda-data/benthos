package eval

import (
	"bytes"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/pratt/syntax"
)

// RegisterLambdaMethods registers methods that take lambda arguments and
// need access to the interpreter for evaluation.
func (interp *Interpreter) RegisterLambdaMethods() {
	lm := func(fn lambdaMethodFunc, params ...MethodParam) MethodSpec {
		return MethodSpec{LambdaFn: fn, Params: params}
	}
	fnParam := MethodParam{Name: "fn"}

	interp.RegisterLambdaMethod("filter", lm(interp.methodFilter, fnParam))
	interp.RegisterLambdaMethod("map", lm(interp.methodMap, fnParam))
	interp.RegisterLambdaMethod("sort", MethodSpec{LambdaFn: interp.methodSort})
	interp.RegisterLambdaMethod("sort_by", lm(interp.methodSortBy, fnParam))
	interp.RegisterLambdaMethod("any", lm(interp.methodAny, fnParam))
	interp.RegisterLambdaMethod("all", lm(interp.methodAll, fnParam))
	interp.RegisterLambdaMethod("find", lm(interp.methodFind, fnParam))
	interp.RegisterLambdaMethod("fold", lm(interp.methodFold, MethodParam{Name: "initial"}, fnParam))
	interp.RegisterLambdaMethod("unique", lm(interp.methodUnique, MethodParam{Name: "fn", HasDefault: true}))
	interp.RegisterLambdaMethod("without_index", lm(interp.methodWithoutIndex, MethodParam{Name: "index"}))
	interp.RegisterLambdaMethod("index_of", lm(interp.methodIndexOf, MethodParam{Name: "target"}))
	interp.RegisterLambdaMethod("slice", lm(interp.methodSlice, MethodParam{Name: "low"}, MethodParam{Name: "high", HasDefault: true}))
	interp.RegisterLambdaMethod("map_values", lm(interp.methodMapValues, fnParam))
	interp.RegisterLambdaMethod("map_keys", lm(interp.methodMapKeys, fnParam))
	interp.RegisterLambdaMethod("map_entries", lm(interp.methodMapEntries, fnParam))
	interp.RegisterLambdaMethod("filter_entries", lm(interp.methodFilterEntries, fnParam))
	interp.RegisterLambdaMethod("into", lm(interp.methodInto, fnParam))
}

// methodInto invokes the lambda with the receiver as its single argument
// and returns the lambda's result. Errors, void, and deleted() from the
// lambda propagate through unchanged — the calling context decides what
// to do with them.
func (interp *Interpreter) methodInto(receiver any, args []syntax.CallArg) any {
	lambda := interp.extractLambdaOrMapRef(args)
	if lambda == nil {
		return NewError("into() requires a lambda argument")
	}
	if len(lambda.Params) != 1 {
		return NewError(fmt.Sprintf("into() requires a one-parameter lambda, got %d parameters", len(lambda.Params)))
	}
	argBuf := [1]any{receiver}
	return interp.callLambda(lambda, argBuf[:])
}

func (interp *Interpreter) methodFilter(receiver any, args []syntax.CallArg) any {
	arr, ok := receiver.([]any)
	if !ok {
		return NewError(fmt.Sprintf("filter() requires array, got %T", receiver))
	}
	lambda := interp.extractLambdaOrMapRef(args)
	if lambda == nil {
		return NewError("filter() requires a lambda argument")
	}
	var argBuf [1]any
	var result []any
	for _, elem := range arr {
		argBuf[0] = elem
		val := interp.callLambda(lambda, argBuf[:])
		if IsError(val) {
			return val
		}
		if IsVoid(val) {
			return NewError("filter() lambda returned void")
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
	var argBuf [1]any
	var result []any
	for _, elem := range arr {
		argBuf[0] = elem
		val := interp.callLambda(lambda, argBuf[:])
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
	if !isSortable(arr[0]) {
		return NewError(fmt.Sprintf("sort(): %T is not a sortable type", arr[0]))
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
	var argBuf [1]any
	keys := make([]any, len(arr))
	for i, elem := range arr {
		argBuf[0] = elem
		key := interp.callLambda(lambda, argBuf[:])
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
	var argBuf [1]any
	for _, elem := range arr {
		argBuf[0] = elem
		val := interp.callLambda(lambda, argBuf[:])
		if IsError(val) {
			return val
		}
		if IsVoid(val) {
			return NewError("any() lambda returned void")
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
	var argBuf [1]any
	for _, elem := range arr {
		argBuf[0] = elem
		val := interp.callLambda(lambda, argBuf[:])
		if IsError(val) {
			return val
		}
		if IsVoid(val) {
			return NewError("all() lambda returned void")
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
	var argBuf [1]any
	for _, elem := range arr {
		argBuf[0] = elem
		val := interp.callLambda(lambda, argBuf[:])
		if IsError(val) {
			return val
		}
		if IsVoid(val) {
			return NewError("find() lambda returned void")
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

	var argBuf2 [2]any
	tally := initial
	for _, elem := range arr {
		argBuf2[0] = tally
		argBuf2[1] = elem
		tally = interp.callLambda(lambda, argBuf2[:])
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

	var argBuf [1]any
	var result []any
	for _, elem := range arr {
		var key any
		if keyFn != nil {
			argBuf[0] = elem
			key = interp.callLambda(keyFn, argBuf[:])
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
	var argBuf [1]any
	result := make(map[string]any, len(obj))
	for k, v := range obj {
		argBuf[0] = v
		val := interp.callLambda(lambda, argBuf[:])
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
	var argBuf [1]any
	result := make(map[string]any, len(obj))
	for k, v := range obj {
		argBuf[0] = k
		newKey := interp.callLambda(lambda, argBuf[:])
		if IsError(newKey) {
			return newKey
		}
		if IsVoid(newKey) {
			return NewError("map_keys() lambda returned void")
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
	var argBuf2 [2]any
	result := make(map[string]any, len(obj))
	for k, v := range obj {
		argBuf2[0] = k
		argBuf2[1] = v
		entry := interp.callLambda(lambda, argBuf2[:])
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
	var argBuf2 [2]any
	result := make(map[string]any, len(obj))
	for k, v := range obj {
		argBuf2[0] = k
		argBuf2[1] = v
		val := interp.callLambda(lambda, argBuf2[:])
		if IsError(val) {
			return val
		}
		if IsVoid(val) {
			return NewError("filter_entries() lambda returned void")
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
// If the argument is a bare identifier or qualified reference (map name),
// synthesizes a lambda that calls the map with a single parameter (Section 5.5).
func (interp *Interpreter) extractLambdaOrMapRef(args []syntax.CallArg) *syntax.LambdaExpr {
	if len(args) == 0 {
		return nil
	}

	// Direct lambda.
	if lambda, ok := args[0].Value.(*syntax.LambdaExpr); ok {
		return lambda
	}

	// Bare identifier or qualified reference → map name reference.
	if ident, ok := args[0].Value.(*syntax.IdentExpr); ok {
		if ident.Namespace != "" {
			// Qualified reference: namespace::name
			return interp.synthesizeNamespacedMapLambda(ident)
		}
		// Local map reference.
		if m, exists := interp.maps[ident.Name]; exists {
			return interp.synthesizeMapLambda(ident.TokenPos, ident.Name, "", m)
		}
	}

	return nil
}

// synthesizeMapLambda creates a lambda that calls the given map with a single
// argument. Returns nil if the map doesn't accept exactly 1 required param.
func (interp *Interpreter) synthesizeMapLambda(pos syntax.Pos, name, namespace string, m *syntax.MapDecl) *syntax.LambdaExpr {
	required := 0
	for _, p := range m.Params {
		if p.Default == nil && !p.Discard {
			required++
		}
	}
	if required != 1 {
		return nil // will trigger "requires a lambda argument" error
	}
	return &syntax.LambdaExpr{
		TokenPos: pos,
		Params:   []syntax.Param{{Name: "__arg", Pos: pos}},
		Body: &syntax.ExprBody{
			Result: &syntax.CallExpr{
				TokenPos:  pos,
				Namespace: namespace,
				Name:      name,
				Args:      []syntax.CallArg{{Value: &syntax.IdentExpr{TokenPos: pos, Name: "__arg"}}},
			},
		},
	}
}

// synthesizeNamespacedMapLambda looks up a qualified map reference and
// synthesizes a lambda for it.
func (interp *Interpreter) synthesizeNamespacedMapLambda(ident *syntax.IdentExpr) *syntax.LambdaExpr {
	ns, ok := interp.namespaces[ident.Namespace]
	if !ok {
		return nil
	}
	m, ok := ns[ident.Name]
	if !ok {
		return nil
	}
	return interp.synthesizeMapLambda(ident.TokenPos, ident.Name, ident.Namespace, m)
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

	// Numeric comparison with checked promotion.
	if isNumeric(a) && isNumeric(b) {
		pl, pr, kind, promErr := promoteChecked(a, b)
		if promErr != "" {
			return NewError(promErr)
		}
		switch kind {
		case promoteInt64:
			return cmpOrdered(pl.(int64), pr.(int64))
		case promoteInt32:
			return cmpOrdered(int64(pl.(int32)), int64(pr.(int32)))
		case promoteUint32:
			return cmpOrdered(uint64(pl.(uint32)), uint64(pr.(uint32)))
		case promoteUint64:
			return cmpOrdered(pl.(uint64), pr.(uint64))
		case promoteFloat64:
			av, bv := pl.(float64), pr.(float64)
			if av < bv {
				return int64(-1)
			}
			if av > bv {
				return int64(1)
			}
			return int64(0)
		case promoteFloat32:
			av, bv := float64(pl.(float32)), float64(pr.(float32))
			if av < bv {
				return int64(-1)
			}
			if av > bv {
				return int64(1)
			}
			return int64(0)
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

type cmpOrderable interface {
	~int64 | ~uint64
}

func cmpOrdered[T cmpOrderable](a, b T) int64 {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func isSortable(v any) bool {
	switch v.(type) {
	case int32, int64, uint32, uint64, float32, float64, string, time.Time:
		return true
	default:
		return false
	}
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
