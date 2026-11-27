// Copyright 2026 Redpanda Data, Inc.

package service_test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/public/service"
	"github.com/redpanda-data/benthos/v4/public/utils/ordering"

	_ "github.com/redpanda-data/benthos/v4/public/components/pure"
)

// orderedGateInput yields its messages in fixed size batches and, like an
// input that preserves ordering, holds the next batch back until every message
// of the previous one has either been settled or been acknowledged.
type orderedGateInput struct {
	total     int
	batchSize int

	// retryNacks makes the input deliver a nacked batch again.
	retryNacks bool

	next    int
	gate    chan struct{}
	yielded atomic.Int64
	settled atomic.Int64

	acked   atomic.Int64
	allDone chan struct{}
}

func newOrderedGateInput(total, batchSize int) *orderedGateInput {
	return &orderedGateInput{
		total:     total,
		batchSize: batchSize,
		gate:      make(chan struct{}, 1),
		allDone:   make(chan struct{}),
	}
}

func (o *orderedGateInput) Connect(context.Context) error { return nil }

func (o *orderedGateInput) ReadBatch(ctx context.Context) (service.MessageBatch, service.AckFunc, error) {
	if o.next >= o.total {
		select {
		case <-o.allDone:
			return nil, nil, service.ErrEndOfInput
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		}
	}

	select {
	case o.gate <- struct{}{}:
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}

	n := min(o.batchSize, o.total-o.next)

	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() { <-o.gate })
	}

	pendingSettled := atomic.Int64{}
	pendingSettled.Store(int64(n))

	batch := make(service.MessageBatch, n)
	for i := range n {
		batch[i] = ordering.OnSettled(service.NewMessage([]byte(strconv.Itoa(o.next+i))), func() {
			o.settled.Add(1)
			if pendingSettled.Add(-1) == 0 {
				release()
			}
		})
	}
	o.next += n
	o.yielded.Add(1)

	return batch, func(context.Context, error) error {
		release()
		if o.acked.Add(int64(n)) == int64(o.total) {
			close(o.allDone)
		}
		return nil
	}, nil
}

func (o *orderedGateInput) Close(context.Context) error { return nil }

// recordingGateOutput records the batches it is given. When block is set each
// write waits to be released. The first failWrites writes return an error.
// When delay is set each write takes that long, less for each later write, so
// that writes in flight together would finish out of order.
type recordingGateOutput struct {
	failWrites int
	delay      time.Duration

	inFlight    int
	maxInFlight int

	block   bool
	started chan struct{}
	release chan struct{}

	mut     sync.Mutex
	batches [][]string
}

func (r *recordingGateOutput) Connect(context.Context) error { return nil }

func (r *recordingGateOutput) WriteBatch(ctx context.Context, batch service.MessageBatch) error {
	contents := make([]string, len(batch))
	for i, m := range batch {
		b, err := m.AsBytes()
		if err != nil {
			return err
		}
		contents[i] = string(b)
	}
	if r.delay > 0 {
		r.mut.Lock()
		r.inFlight++
		r.maxInFlight = max(r.maxInFlight, r.inFlight)
		delay := r.delay / time.Duration(len(r.batches)+1)
		r.mut.Unlock()

		defer func() {
			r.mut.Lock()
			r.inFlight--
			r.mut.Unlock()
		}()

		// The batch is recorded below, once the write has taken its time.
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	r.mut.Lock()
	r.batches = append(r.batches, contents)
	fail := len(r.batches) <= r.failWrites
	r.mut.Unlock()

	if r.block {
		select {
		case r.started <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}
		select {
		case <-r.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if fail {
		return errors.New("write failed")
	}
	return nil
}

func (r *recordingGateOutput) Close(context.Context) error { return nil }

func (r *recordingGateOutput) mostInFlight() int {
	r.mut.Lock()
	defer r.mut.Unlock()
	return r.maxInFlight
}

func (r *recordingGateOutput) written() [][]string {
	r.mut.Lock()
	defer r.mut.Unlock()
	return append([][]string{}, r.batches...)
}

func runOrderedGateStream(t *testing.T, in *orderedGateInput, out *recordingGateOutput, conf string) (wait func()) {
	t.Helper()

	env := service.NewEnvironment()
	require.NoError(t, env.RegisterBatchInput("ordered_gate", service.NewConfigSpec(),
		func(*service.ParsedConfig, *service.Resources) (service.BatchInput, error) {
			if in.retryNacks {
				return service.AutoRetryNacksBatched(in), nil
			}
			return in, nil
		}))
	require.NoError(t, env.RegisterBatchOutput("recording_gate",
		service.NewConfigSpec().
			Field(service.NewOutputMaxInFlightField()).
			Field(service.NewBatchPolicyField("batching")),
		func(conf *service.ParsedConfig, _ *service.Resources) (o service.BatchOutput, pol service.BatchPolicy, mif int, err error) {
			if pol, err = conf.FieldBatchPolicy("batching"); err != nil {
				return
			}
			if mif, err = conf.FieldMaxInFlight(); err != nil {
				return
			}
			return out, pol, mif, nil
		}))

	builder := env.NewStreamBuilder()
	require.NoError(t, builder.SetYAML(conf))
	require.NoError(t, builder.SetLoggerYAML(`level: OFF`))

	stream, err := builder.Build()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(t.Context(), time.Second*30)
	t.Cleanup(cancel)

	errChan := make(chan error, 1)
	go func() {
		errChan <- stream.Run(ctx)
	}()
	return func() {
		t.Helper()
		select {
		case err := <-errChan:
			require.NoError(t, err)
		case <-time.After(time.Second * 30):
			t.Fatalf("stream did not finish, the input stalled after yielding %v batches", in.yielded.Load())
		}
	}
}

func seq(from, to int) []string {
	out := make([]string, 0, to-from)
	for i := from; i < to; i++ {
		out = append(out, strconv.Itoa(i))
	}
	return out
}

func suffixed(in []string, suffix string) []string {
	for i := range in {
		in[i] += suffix
	}
	return in
}

// An input that waits for settling or acknowledgement before yielding more
// must not be throttled to one of its own batches per flush of the batch
// policy of an output with a single writer. Each config batches far more
// messages than the input yields at a time, and no batch is flushed by a
// period within the test, so the stream only finishes if joining a batch
// releases the input.
func TestOrderingSettledReleasesGatedInputIntoBatchers(t *testing.T) {
	tests := []struct {
		name string
		conf string
		exp  [][]string
	}{
		{
			name: "output batching",
			conf: `
input:
  ordered_gate: {}
pipeline:
  threads: 4
output:
  recording_gate:
    max_in_flight: 1
    batching:
      count: 50
      period: 1h
`,
			exp: [][]string{seq(0, 50), seq(50, 100)},
		},
		{
			name: "output batching with processors",
			conf: `
input:
  ordered_gate: {}
pipeline:
  threads: 4
output:
  recording_gate:
    max_in_flight: 1
    batching:
      count: 50
      period: 1h
      processors:
        - archive:
            format: lines
`,
			exp: [][]string{
				{strings.Join(seq(0, 50), "\n")},
				{strings.Join(seq(50, 100), "\n")},
			},
		},
		{
			// Ordering is not guaranteed across several outputs, so one of
			// them batching a message releases the input for all of them.
			name: "output batching in a fan out",
			conf: `
input:
  ordered_gate: {}
output:
  broker:
    pattern: fan_out
    outputs:
      - recording_gate:
          max_in_flight: 1
          batching:
            count: 50
            period: 1h
      - drop: {}
`,
			exp: [][]string{seq(0, 50), seq(50, 100)},
		},
		{
			name: "output batching in a sequential fan out",
			conf: `
input:
  ordered_gate: {}
output:
  broker:
    pattern: fan_out_sequential
    outputs:
      - recording_gate:
          max_in_flight: 1
          batching:
            count: 50
            period: 1h
      - drop: {}
`,
			exp: [][]string{seq(0, 50), seq(50, 100)},
		},
		{
			name: "output batching in a switch that continues",
			conf: `
input:
  ordered_gate: {}
output:
  switch:
    cases:
      - continue: true
        output:
          recording_gate:
            max_in_flight: 1
            batching:
              count: 50
              period: 1h
      - output:
          drop: {}
`,
			exp: [][]string{seq(0, 50), seq(50, 100)},
		},
		{
			name: "output batching behind a resource",
			conf: `
input:
  ordered_gate: {}
output:
  resource: batched
output_resources:
  - label: batched
    recording_gate:
      max_in_flight: 1
      batching:
        count: 50
        period: 1h
`,
			exp: [][]string{seq(0, 50), seq(50, 100)},
		},
		{
			name: "output batching behind wrapping outputs",
			conf: `
input:
  ordered_gate: {}
output:
  fallback:
    - switch:
        cases:
          - check: content() == "nope"
            output:
              drop: {}
          - output:
              broker:
                pattern: fan_out
                outputs:
                  - recording_gate:
                      max_in_flight: 1
                      batching:
                        count: 50
                        period: 1h
                    processors:
                      - mapping: 'root = content().string() + "!"'
    - drop: {}
`,
			exp: [][]string{suffixed(seq(0, 50), "!"), suffixed(seq(50, 100), "!")},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			in := newOrderedGateInput(100, 5)
			out := &recordingGateOutput{}

			wait := runOrderedGateStream(t, in, out, test.conf)
			wait()

			assert.Equal(t, test.exp, out.written())
		})
	}
}

// An output without a batch policy does not settle a batch when it takes it,
// whatever its max_in_flight, so the input stays gated on the acknowledgement
// and keeps its order through a failed write.
func TestOrderingKeepsGatedInputBehindWriters(t *testing.T) {
	for _, maxInFlight := range []int{1, 4} {
		t.Run(fmt.Sprintf("max_in_flight %v", maxInFlight), func(t *testing.T) {
			in := newOrderedGateInput(20, 5)
			out := &recordingGateOutput{
				block:   true,
				started: make(chan struct{}),
				release: make(chan struct{}),
			}

			wait := runOrderedGateStream(t, in, out, fmt.Sprintf(`
input:
  ordered_gate: {}
output:
  recording_gate:
    max_in_flight: %v
`, maxInFlight))

			for i := range 4 {
				select {
				case <-out.started:
				case <-time.After(time.Second * 10):
					t.Fatal("timed out")
				}

				time.Sleep(time.Millisecond * 50)
				require.Equal(t, int64(i+1), in.yielded.Load(), "write %v", i)

				out.release <- struct{}{}
			}
			wait()

			assert.Equal(t, [][]string{seq(0, 5), seq(5, 10), seq(10, 15), seq(15, 20)}, out.written())
		})
	}
}

// With a single writer the batches of an input that was released early land
// in the order they were flushed.
func TestOrderingKeptAcrossBatchesWithSingleWriter(t *testing.T) {
	in := newOrderedGateInput(400, 5)
	out := &recordingGateOutput{delay: time.Millisecond * 20}

	wait := runOrderedGateStream(t, in, out, `
input:
  ordered_gate: {}
pipeline:
  threads: 4
output:
  recording_gate:
    max_in_flight: 1
    batching:
      count: 20
`)
	wait()

	var exp [][]string
	for i := 0; i < 400; i += 20 {
		exp = append(exp, seq(i, i+20))
	}
	assert.Equal(t, exp, out.written())
	assert.Equal(t, 1, out.mostInFlight())
}

// A batch policy only settles ordering when its output is known to have a
// single writer. Otherwise batches could be written out of order, so the input
// stays gated on the acknowledgement. Each config needs far more messages than
// the input yields at a time and never flushes on a period, so nothing is
// written.
func TestOrderingNotSettledWithoutSingleWriter(t *testing.T) {
	tests := []struct {
		name string
		conf string
	}{
		{
			name: "output with parallel writers",
			conf: `
input:
  ordered_gate: {}
output:
  recording_gate:
    max_in_flight: 2
    batching:
      count: 50
`,
		},
		{
			// A broker does not know how its outputs write.
			name: "batch policy on a broker",
			conf: `
input:
  ordered_gate: {}
output:
  broker:
    batching:
      count: 50
    outputs:
      - recording_gate:
          max_in_flight: 1
`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			in := newOrderedGateInput(100, 5)
			out := &recordingGateOutput{}

			runOrderedGateStream(t, in, out, test.conf)

			require.Eventually(t, func() bool {
				return in.yielded.Load() == 1
			}, time.Second*5, time.Millisecond)
			time.Sleep(time.Millisecond * 200)
			assert.Equal(t, int64(1), in.yielded.Load())
			assert.Equal(t, int64(0), in.settled.Load())
			assert.Empty(t, out.written())
		})
	}
}

// A message that a processor filters never joins a batch, so its ordering is
// never settled and the input waits for the acknowledgement of that input
// batch. The messages that survive are settled as usual.
func TestOrderingPartialFilterKeepsInputGated(t *testing.T) {
	in := newOrderedGateInput(100, 5)
	out := &recordingGateOutput{}

	runOrderedGateStream(t, in, out, `
input:
  ordered_gate: {}
pipeline:
  processors:
    - mapping: 'root = if content().number() % 2 == 1 { deleted() }'
output:
  recording_gate:
    max_in_flight: 1
    batching:
      count: 50
`)

	// Of the first input batch, 0 to 4, three messages survive.
	require.Eventually(t, func() bool {
		return in.settled.Load() == 3
	}, time.Second*5, time.Millisecond)
	time.Sleep(time.Millisecond * 200)
	assert.Equal(t, int64(1), in.yielded.Load())
	assert.Equal(t, int64(3), in.settled.Load())
	assert.Empty(t, out.written())
}

// An input batch that is filtered entirely is acknowledged straight away,
// which releases the input without any ordering being settled.
func TestOrderingFullyFilteredBatchesReleasedByAck(t *testing.T) {
	in := newOrderedGateInput(100, 5)
	out := &recordingGateOutput{}

	wait := runOrderedGateStream(t, in, out, `
input:
  ordered_gate: {}
pipeline:
  processors:
    - mapping: 'root = deleted()'
output:
  recording_gate:
    max_in_flight: 1
    batching:
      count: 50
`)
	wait()

	assert.Equal(t, int64(20), in.yielded.Load())
	assert.Equal(t, int64(0), in.settled.Load())
	assert.Empty(t, out.written())
}

// A batch that fails after its messages were settled is delivered again
// behind the batches the input released in the meantime.
func TestOrderingFailedBatchRetriedBehindLaterBatches(t *testing.T) {
	in := newOrderedGateInput(100, 5)
	in.retryNacks = true
	out := &recordingGateOutput{
		failWrites: 1,
		block:      true,
		started:    make(chan struct{}),
		release:    make(chan struct{}),
	}

	wait := runOrderedGateStream(t, in, out, `
input:
  ordered_gate: {}
output:
  recording_gate:
    max_in_flight: 1
    batching:
      count: 50
`)

	waitForWrite := func() {
		t.Helper()
		select {
		case <-out.started:
		case <-time.After(time.Second * 10):
			t.Fatal("timed out")
		}
	}

	// Hold the first write until every message has joined a batch, so that
	// the second batch is waiting for the writer when the first one fails.
	waitForWrite()
	require.Eventually(t, func() bool {
		return in.settled.Load() == 100
	}, time.Second*5, time.Millisecond)
	out.release <- struct{}{}

	for range 2 {
		waitForWrite()
		out.release <- struct{}{}
	}
	wait()

	written := out.written()
	require.Len(t, written, 3)
	assert.Equal(t, seq(0, 50), written[0], "the write that fails")
	assert.Equal(t, seq(50, 100), written[1])
	assert.ElementsMatch(t, seq(0, 50), written[2], "the retry of the failed batch")
}

// ExecuteProcessors hands back messages that still carry the ordering
// callbacks of the messages it was given, so a component that runs nested
// processors and forwards the results does not cut the callbacks off.
func TestExecuteProcessorsKeepsOrderingCallbacks(t *testing.T) {
	conf, err := service.NewConfigSpec().
		Field(service.NewProcessorListField("processors")).
		ParseYAML(`
processors:
  - mapping: 'root = content().uppercase()'
`, nil)
	require.NoError(t, err)

	procs, err := conf.FieldProcessorList("processors")
	require.NoError(t, err)
	t.Cleanup(func() {
		for _, p := range procs {
			require.NoError(t, p.Close(context.Background()))
		}
	})

	fired := make([]int, 4)
	batch := make(service.MessageBatch, len(fired))
	for i := range fired {
		batch[i] = ordering.OnSettled(service.NewMessage([]byte(strconv.Itoa(i))), func() {
			fired[i]++
		})
	}

	res, err := service.ExecuteProcessors(t.Context(), procs, batch)
	require.NoError(t, err)
	require.Len(t, res, 1)
	require.Len(t, res[0], len(fired))
	assert.Equal(t, []int{0, 0, 0, 0}, fired)

	for _, m := range res[0] {
		ordering.Settled(m)
	}
	assert.Equal(t, []int{1, 1, 1, 1}, fired)
}
