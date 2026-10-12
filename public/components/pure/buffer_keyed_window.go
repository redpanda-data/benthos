package pure

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/redpanda-data/benthos/v4/internal/batch"
	"github.com/redpanda-data/benthos/v4/internal/value"
	"github.com/redpanda-data/benthos/v4/public/bloblang"
	"github.com/redpanda-data/benthos/v4/public/service"
)

const (
	kwbFieldTimestampMapping = "timestamp_mapping"
	kwbFieldMaxPendingKeys   = "max_pending_keys"
	kwbFieldTimeout          = "timeout"
	kwbFieldKeyMapping       = "key_mapping"
	kwbFieldLengthMapping    = "length_mapping"
	kwbFieldCheckMapping     = "check"
)

func keyedWindowBufferConfig() *service.ConfigSpec {
	return service.NewConfigSpec().
		Beta().
		Version("4.26.0").
		Categories("Windowing").
		Summary("Chops a stream of messages into keyed windows of with a batch timeout, following the system clock.").
		Description(`
== Metadata

This buffer adds the following metadata fields to each message:

`+"```text"+`
- batch_key
- batch_expected_length (if set, else -1)
- batch_length
`+"```"+`

You can access these metadata fields using
xref:configuration:interpolation.adoc#bloblang-queries[function interpolation].

The beginning of the window for each key starts at the time of the first message for that key, and the window is closed after the specified duration has passed or when the the check mappping returns true.

## Grouping Messages

Grouping of messages is achieved by specifying a xref:guides:bloblang/about.adoc[Bloblang mapping] that extracts a key from each message. Messages with the same key are grouped together into a buffer.

## Back Pressure

Due to the nature of this buffer, it is possible for unbounded memory usage to occur if the size of the time window or the size of the maxiumum pending keys is too large.  Ensure that you have enough memory allocated to cover the potential number of messages that could be buffered at any given time.

## Delivery Guarantees

This buffer honours the transaction model within Benthos in order to ensure that messages are not acknowledged until they are either intentionally dropped or successfully delivered to outputs. However, if  messages are delivered after the window has closed, it will start a new buffer for that key.

During graceful termination any buffer is partially populated with messages they will be nacked such that they are re-consumed the next time the service starts.
`).
		Field(service.NewBloblangField(kwbFieldTimestampMapping).
			Description(`
A xref:guides:bloblang/about.adoc[Bloblang mapping] applied to each message during ingestion that provides the timestamp to use for allocating it a window. By default the function `+"`now()`"+` is used in order to generate a fresh timestamp at the time of ingestion (the processing time), whereas this mapping can instead extract a timestamp from the message itself (the event time).

The timestamp value assigned to `+"`root`"+` must either be a numerical unix time in seconds (with up to nanosecond precision via decimals), or a string in ISO 8601 format. If the mapping fails or provides an invalid result the message will be dropped (with logging to describe the problem).
`).
			Default("root = now()").
			Example("root = this.created_at").Example(`root = metadata("kafka_timestamp_unix").number()`)).
		Field(service.NewIntField(kwbFieldMaxPendingKeys).
			Description("The maximum number of pending keys allowed, if the maximum is hit any messages not belonging to an existing key will be nacked until some items are cleared.  Set to zero for no limit.").
			Default(100)).
		Field(service.NewStringField(kwbFieldTimeout).
			Description("A duration string describing the maximum size of each window. After the time limit has passed, the buffer will be closed and grouped messages will be sent.").
			Example("30s").Example("10m")).
		Field(service.NewInterpolatedStringField(kwbFieldKeyMapping).
			Description("The interpolated string to batch based on.").
			Examples("${! metadata(\"kafka_key\") }", "${! json(\"foo.bar\") }-${! metadata(\"baz\") }")).
		Field(service.NewInterpolatedStringField(kwbFieldLengthMapping).
			Description("An optional interpolated string that extracts the expected length of the batch. This is used to preemptively allocate memory for the buffer.").
			Examples("${! json(\"message.count\") }").
			Optional().Default("")).
		Field(service.NewStringField(kwbFieldCheckMapping).
			Description("An optional xref:guides:bloblang/about.adoc[Bloblang mapping] that returns a boolean value. If the mapping returns true the window will be closed and the grouped messages will be sent.").
			Examples("meta('batch_length') >= meta('batch_expected_length')").
			Optional().Default("")).
		Example("Grouping messages by sequence id", `Given a stream of messages that are linked by a sequence id of the form:

`+"```json"+`
{
  "sequence_id": "sequence_1",
  "created_at": "2021-08-07T09:49:35Z",
  "field_a": "field a value",
  "field_b": "field b value"
}
`+"```"+`

We can use a keyed window buffer in order to group multiple messages for the same sequence ID into a single message of this form:

`+"```json"+`
{
  "sequence_id": "sequence_1",
  "created_at": "2021-08-07T10:00:00Z",
  "field_a": "field a value",
  "field_b": "field b value",
  "field_c": "field c value",
}
`+"```"+`

With the following config:`,
			`
buffer:
  keyed_window:
    key_mapping: '${! json("sequence_id") }'
    timeout: 10s
    max_pending_keys: 10

pipeline:
  processors:

    # Reduce each batch to a single message by deleting indexes > 0, and
    # squash into a single object
    - mapping: |
        if batch_index() == 0 {
            root = json("").from_all().enumerated().map_each(item -> if item.index == 0 { item.value } else { item.value.without("sequence_id") }).squash()
        }
        else {
            root = deleted()
        }
`,
		)
}

func init() {
	err := service.RegisterBatchBuffer(
		"keyed_window", keyedWindowBufferConfig(),
		func(conf *service.ParsedConfig, mgr *service.Resources) (service.BatchBuffer, error) {
			return newKeyedWindowBufferFromConf(conf, mgr, nil)
		})
	if err != nil {
		panic(err)
	}
}

//------------------------------------------------------------------------------

func getDuration(conf *service.ParsedConfig, required bool, name string) (time.Duration, error) {
	periodStr, err := conf.FieldString(name)
	if err != nil {
		return 0, err
	}
	if !required && periodStr == "" {
		return 0, nil
	}
	period, err := time.ParseDuration(periodStr)
	if err != nil {
		return 0, fmt.Errorf("failed to parse field '%v' as duration: %w", name, err)
	}
	return period, nil
}

type utcNowProvider func() time.Time

type ackMessage struct {
	m     *service.Message
	ackFn service.AckFunc
}

type keyedWindowBatch struct {
	messages    []*ackMessage
	key         string
	expiry      time.Time
	queued      bool
	passesCheck bool
}

func (k *keyedWindowBatch) IsExpired(
	clock utcNowProvider,
) bool {
	if clock == nil {
		clock = func() time.Time {
			return time.Now().UTC()
		}
	}
	return clock().After(k.expiry)
}

type keyedWindowBuffer struct {
	logger *service.Logger

	tsMapping, check          *bloblang.Executor
	lengthMapping, keyMapping *service.InterpolatedString
	clock                     utcNowProvider
	size                      time.Duration
	maxPendingKeys            int

	pending    map[string]*keyedWindowBatch
	pendingMut sync.Mutex

	keyCompletedChan   chan string
	refreshTimeoutChan chan struct{}

	endOfInputChan      chan struct{}
	closeEndOfInputOnce sync.Once
}

func newKeyedWindowBufferFromConf(conf *service.ParsedConfig, res *service.Resources, timeProvider utcNowProvider) (*keyedWindowBuffer, error) {
	size, err := getDuration(conf, true, kwbFieldTimeout)
	if err != nil {
		return nil, err
	}
	maxPendingKeys, err := conf.FieldInt(kwbFieldMaxPendingKeys)
	if err != nil {
		return nil, err
	}
	tsMapping, err := conf.FieldBloblang(kwbFieldTimestampMapping)
	if err != nil {
		return nil, err
	}
	lengthMapping, err := conf.FieldInterpolatedString(kwbFieldLengthMapping)
	if err != nil {
		return nil, err
	}
	keyMapping, err := conf.FieldInterpolatedString(kwbFieldKeyMapping)
	if err != nil {
		return nil, err
	}
	checkStr, err := conf.FieldString(kwbFieldCheckMapping)
	if err != nil {
		return nil, err
	}

	var check *bloblang.Executor
	if len(checkStr) > 0 {
		check, err = conf.FieldBloblang("check")
		if err != nil {
			return nil, err
		}
	}

	if timeProvider == nil {
		timeProvider = func() time.Time {
			return time.Now().UTC()
		}
	}

	return newKeyedWindowBuffer(tsMapping, keyMapping, lengthMapping, check, maxPendingKeys, timeProvider, size, res)
}

func newKeyedWindowBuffer(
	tsMapping *bloblang.Executor,
	keyMapping, lengthMapping *service.InterpolatedString,
	check *bloblang.Executor,
	maxPendingKeys int,
	clock utcNowProvider,
	size time.Duration,
	res *service.Resources,
) (*keyedWindowBuffer, error) {

	w := &keyedWindowBuffer{
		tsMapping:      tsMapping,
		keyMapping:     keyMapping,
		lengthMapping:  lengthMapping,
		maxPendingKeys: maxPendingKeys,
		check:          check,
		clock:          clock,
		size:           size,
		logger:         res.Logger(),
		endOfInputChan: make(chan struct{}),
		pending:        map[string]*keyedWindowBatch{},
	}

	w.refreshTimeoutChan = make(chan struct{})
	tmpKeyCompletedChan := make(chan string)
	w.keyCompletedChan = tmpKeyCompletedChan
	go func() {
		w.queueTimeouts()
	}()
	return w, nil
}

func (w *keyedWindowBuffer) getTimestamp(i int, batch service.MessageBatch) (ts time.Time, err error) {
	var tsValueMsg *service.Message
	if tsValueMsg, err = batch.BloblangQuery(i, w.tsMapping); err != nil {
		w.logger.Errorf("Timestamp mapping failed for message: %v", err)
		err = fmt.Errorf("timestamp mapping failed: %w", err)
		return
	}

	var tsValue any
	if tsValue, err = tsValueMsg.AsStructured(); err != nil {
		if tsBytes, _ := tsValueMsg.AsBytes(); len(tsBytes) > 0 {
			tsValue = string(tsBytes)
			err = nil
		}
	}
	if err != nil {
		w.logger.Errorf("Timestamp mapping failed for message: unable to parse result as structured value: %v", err)
		err = fmt.Errorf("unable to parse result of timestamp mapping as structured value: %w", err)
		return
	}

	if ts, err = value.IGetTimestamp(tsValue); err != nil {
		w.logger.Errorf("Timestamp mapping failed for message: %v", err)
		err = fmt.Errorf("unable to parse result of timestamp mapping as timestamp: %w", err)
	}
	return
}

func (w *keyedWindowBuffer) WriteBatch(ctx context.Context, msgBatch service.MessageBatch, aFn service.AckFunc) error {
	aggregatedAck := batch.NewCombinedAcker(batch.AckFunc(aFn))

	// And now add new messages.
	for i, msg := range msgBatch {
		out, _ := msg.AsStructured()
		w.logger.Infof("Processing message: %v %v", i, out)
		ts, err := w.getTimestamp(i, msgBatch)
		if err != nil {
			_ = aFn(ctx, fmt.Errorf("failed to extract timestamp: %w", err))
			return err
		}

		keyValue, err := msgBatch.TryInterpolatedString(i, w.keyMapping)
		if err != nil {
			_ = aFn(ctx, fmt.Errorf("failed to extract key: %w", err))
			return err
		}

		lengthValue, err := msgBatch.TryInterpolatedString(i, w.lengthMapping)
		if err != nil {
			_ = aFn(ctx, fmt.Errorf("failed to extract length: %w", err))
			return err
		}

		lengthInt := 0
		if len(lengthValue) > 0 {
			lengthInt, err = strconv.Atoi(lengthValue)
			if err != nil {
				_ = aFn(ctx, fmt.Errorf("failed to parse length: %w", err))
				lengthInt = 0
			}
		}
		w.pendingMut.Lock()
		batch, exists := w.pending[keyValue]
		if !exists {
			if w.maxPendingKeys > 0 && len(w.pending) >= w.maxPendingKeys {
				err := fmt.Errorf("max pending keys reached: %v", w.maxPendingKeys)
				_ = aFn(ctx, err)
				w.pendingMut.Unlock()
				return err
			}
			batch = &keyedWindowBatch{
				key:         keyValue,
				expiry:      ts.Add(w.size),
				passesCheck: false,
			}
			w.pending[keyValue] = batch

			// If we have a length mapping then we need to set the length of the
			// batch to the result of the mapping.
			if len(lengthValue) > 0 {
				batch.messages = make([]*ackMessage, 0, lengthInt)
			}
		}
		w.pendingMut.Unlock()

		batch.messages = append(batch.messages, &ackMessage{
			m:     msg,
			ackFn: service.AckFunc(aggregatedAck.Derive()),
		})

		// If we have a check mapping then we need to set the check result of
		// the batch to the result of the mapping.
		if w.check != nil {
			checkBatch := make(service.MessageBatch, 0, len(batch.messages))
			for _, m := range batch.messages {
				newMessage := m.m.Copy()
				newMessage.MetaSetMut("batch_key", keyValue)
				newMessage.MetaSetMut("batch_expected_length", lengthInt)
				newMessage.MetaSetMut("batch_length", len(batch.messages))
				checkBatch = append(checkBatch, newMessage)
			}

			checkValue, err := checkBatch.BloblangQuery(i, w.check)

			if err != nil {
				_ = aFn(ctx, fmt.Errorf("check mapping failed: %w", err))
				return err
			}

			if checkValue != nil {
				newValue, err := checkValue.AsStructured()
				if err != nil {
					batch.passesCheck = false
				}
				if b, ok := newValue.(bool); ok {
					batch.passesCheck = b
					if b {
						w.keyCompletedChan <- keyValue
					}
				} else {
					batch.passesCheck = false
				}
			}
		}
	}

	return nil
}

func (w *keyedWindowBuffer) flushBatch(ctx context.Context, closeKey string) (service.MessageBatch, service.AckFunc, error) {
	w.pendingMut.Lock()
	defer w.pendingMut.Unlock()

	var batch service.MessageBatch
	var flushAcks []service.AckFunc

	if len(w.pending) == 0 {
		return nil, nil, nil
	}

	// If we're not closing a key then return
	if len(closeKey) == 0 {
		return nil, nil, nil
	}

	//
	if pending, exists := w.pending[closeKey]; exists {
		for _, m := range pending.messages {
			tmpMsg := m.m.Copy()
			batch = append(batch, tmpMsg)
			flushAcks = append(flushAcks, m.ackFn)
		}
		delete(w.pending, closeKey)
	} else {
		return nil, nil, nil
	}

	return batch, func(ctx context.Context, err error) error {
		for _, aFn := range flushAcks {
			_ = aFn(ctx, err)
		}
		return nil
	}, nil

}

func (w *keyedWindowBuffer) queueTimeouts() {
	defer close(w.refreshTimeoutChan)
	for {
		select {
		case <-time.After(w.size / 2):
		case <-w.refreshTimeoutChan:
		case <-w.endOfInputChan:
			return
		}
		w.pendingMut.Lock()
		for key, pending := range w.pending {
			if pending.queued {
				continue
			}
			if pending.IsExpired(w.clock) {
				pending.queued = true
				go func(key string) {
					w.keyCompletedChan <- key
				}(key)
			}
		}
		w.pendingMut.Unlock()
	}
}

var errKeyedWindowClosed = errors.New("message rejected as window did not complete")

func (w *keyedWindowBuffer) ReadBatch(ctx context.Context) (service.MessageBatch, service.AckFunc, error) {

	for {
		var key string

		select {
		case key = <-w.keyCompletedChan:
		case <-ctx.Done():
			w.EndOfInput()
			return nil, nil, ctx.Err()
		case <-w.endOfInputChan:
			// Nack all pending messages so that we re-consume them on the next
			// start up. TODO: Eventually allow users to customize this as they
			// may wish to flush partial windows instead.

			w.pendingMut.Lock()
			for _, pending := range w.pending {
				for _, m := range pending.messages {
					_ = m.ackFn(ctx, errKeyedWindowClosed)
				}
			}
			w.pending = nil
			w.pendingMut.Unlock()
			return nil, nil, service.ErrEndOfBuffer
		}
		if msgBatch, aFn, err := w.flushBatch(ctx, key); len(msgBatch) > 0 || err != nil {
			return msgBatch, aFn, err
		}
	}
}

func (w *keyedWindowBuffer) EndOfInput() {
	w.closeEndOfInputOnce.Do(func() {
		close(w.endOfInputChan)
	})
}

func (w *keyedWindowBuffer) Close(ctx context.Context) error {
	return nil
}
