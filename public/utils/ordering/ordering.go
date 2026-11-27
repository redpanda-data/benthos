// Copyright 2026 Redpanda Data, Inc.

// Package ordering lets the components that take a message tell the input it
// came from that the ordering of the message is settled. That is the case
// once the place of the message in what the output will send no longer
// depends on the messages behind it being held back: the message has joined
// a batch of an output that writes its batches in order, an output has handed
// it to a client that sends in order, or the output gives no ordering
// guarantee to protect. The message has not been delivered yet.
//
// An input that holds messages back in order to preserve their ordering can
// register a callback and release the next ones when it is called, before the
// acknowledgement arrives. The acknowledgement is still the only signal of
// delivery.
//
// The batch policy of an output settles the ordering of a message as it joins
// a batch, but only when the output has a max_in_flight of 1. A single writer
// writes batches in the order they were flushed. With more writers a later
// batch could be written first, so the ordering is not settled and the input
// waits for the acknowledgement.
//
// An input that releases on a callback gives up some of what waiting for the
// acknowledgement gave it:
//
//   - A batch that fails is delivered again behind the messages released
//     since.
//   - Ordering is not kept when messages are brokered across more than one
//     output.
//   - A copy of a message shares its callbacks, so the first copy to be
//     settled speaks for the message. The same goes for the messages that a
//     processor expands a message into.
//   - What is in flight is no longer limited by the waiting, so the input
//     should keep its own bound on the messages it has released that are not
//     yet acknowledged.
//
// A callback is not always called: no component on the path of the message
// may settle its ordering, as when a processor filters the message, or the
// output has no batch policy or more than one writer. The input must still
// release on the acknowledgement.
package ordering

import (
	"sync"

	"github.com/redpanda-data/benthos/v4/internal/message"
	"github.com/redpanda-data/benthos/v4/public/service"
)

// OnSettled returns a copy of the message that calls fn when the ordering of
// the message is settled, alongside any callbacks it already carries. Use the
// returned message in place of the one provided.
//
// fn is called at most once for the message, however many times it is
// delivered. A message that was settled and then nacked does not call it
// again when it is retried, and one that was nacked before it was settled can
// call it on a later delivery. It can be called after the message has been
// acknowledged, as when the message passes through a buffer. fn is called
// from the goroutine of the component that settled the message, so it must
// not block, and the callbacks of different messages can run at the same
// time. A nil fn is ignored.
func OnSettled(msg *service.Message, fn func()) *service.Message {
	if fn == nil {
		return msg
	}
	var once sync.Once
	return msg.WithContext(message.CtxOnOrderingSettled(msg.Context(), func() {
		// Copies of the message share this callback and each one can be
		// settled, so the Once keeps fn to a single call.
		once.Do(fn)
	}))
}

// Settled signals that the ordering of the message is settled. An output
// should call this once it has handed the message to something that preserves
// the order of what it is given, when that happens before the write returns,
// or straight away when what it writes to gives no ordering guarantee.
func Settled(msg *service.Message) {
	message.CtxOrderingSettled(msg.Context())
}
