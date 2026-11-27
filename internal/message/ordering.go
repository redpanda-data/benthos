// Copyright 2026 Redpanda Data, Inc.

package message

import "context"

type orderingCallbacksKeyType int

const orderingCallbacksKey orderingCallbacksKeyType = iota

type orderingCallbacks []func()

// CtxOnOrderingSettled returns a context that carries fn as an ordering
// callback, alongside any callbacks the context already carries.
func CtxOnOrderingSettled(ctx context.Context, fn func()) context.Context {
	existing, _ := ctx.Value(orderingCallbacksKey).(orderingCallbacks)
	// Copy so that sibling contexts derived from the same parent never share a
	// backing array.
	fns := make(orderingCallbacks, len(existing), len(existing)+1)
	copy(fns, existing)
	return context.WithValue(ctx, orderingCallbacksKey, append(fns, fn))
}

// CtxOrderingSettled calls every ordering callback carried by the context, in
// the order they were added.
func CtxOrderingSettled(ctx context.Context) {
	fns, _ := ctx.Value(orderingCallbacksKey).(orderingCallbacks)
	for _, fn := range fns {
		fn()
	}
}

// OrderingSettled calls the ordering callbacks of the part. A callback is
// called again each time.
func OrderingSettled(p *Part) {
	CtxOrderingSettled(p.ctx)
}
