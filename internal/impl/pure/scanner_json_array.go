// Copyright 2025 Redpanda Data, Inc.

package pure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/redpanda-data/benthos/v4/public/service"
)

func jsonArrayScannerSpec() *service.ConfigSpec {
	return service.NewConfigSpec().
		Stable().
		Version("4.57.0").
		Summary("Consumes a stream of one or more JSON elements within a top level array.").
		Description(`Use this scanner to read exports whose top-level JSON structure is an array, such as logs or bulk exports, or to break a large file of many objects into one message per element. The scanner decodes elements one at a time, so it doesn't load the whole array into memory.

The input must be one JSON array, or several arrays one after another, such as ` + "`[1,2][3]`" + `. The scanner fails with an error if the input doesn't start with ` + "`[`" + `, or if anything other than another array follows a closing ` + "`]`" + `. Each element can be any JSON value, and becomes one structured message.`).
		// Just a placeholder empty object as we don't have any fields yet
		Field(service.NewObjectField("").Default(map[string]any{}))
}

func init() {
	service.MustRegisterBatchScannerCreator("json_array", jsonArrayScannerSpec(),
		func(conf *service.ParsedConfig, mgr *service.Resources) (service.BatchScannerCreator, error) {
			return &jsonArrayScannerCreator{}, nil
		})
}

type jsonArrayScannerCreator struct{}

func (js *jsonArrayScannerCreator) Create(rdr io.ReadCloser, aFn service.AckFunc, details *service.ScannerSourceDetails) (service.BatchScanner, error) {
	decoder := json.NewDecoder(rdr)

	if err := readFirstBracket(decoder); err != nil {
		return nil, err
	}

	return service.AutoAggregateBatchScannerAcks(&jsonArrayScanner{
		d: decoder,
		r: rdr,
	}, aFn), nil
}

func readFirstBracket(d *json.Decoder) error {
	// Read opening bracket of array
	token, err := d.Token()
	if err != nil {
		return fmt.Errorf("failed to read opening token: %w", err)
	}

	// Verify it's an array
	if delim, ok := token.(json.Delim); !ok || delim != '[' {
		return fmt.Errorf("expected opening '[' but got %v", token)
	}

	return nil
}

func readNextElement(d *json.Decoder) (any, error) {
	for !d.More() {
		// Read closing bracket
		token, err := d.Token()
		if err != nil {
			return nil, fmt.Errorf("failed to read closing token: %w", err)
		}

		if delim, ok := token.(json.Delim); !ok || delim != ']' {
			return nil, fmt.Errorf("expected closing ']' but got %v", token)
		}

		// See if there's another array following
		if err := readFirstBracket(d); err != nil {
			if errors.Is(err, io.EOF) {
				err = io.EOF
			}
			return nil, err
		}
	}

	var element json.RawMessage
	if err := d.Decode(&element); err != nil {
		return nil, fmt.Errorf("failed to decode element: %w", err)
	}

	return element, nil
}

func (js *jsonArrayScannerCreator) Close(context.Context) error {
	return nil
}

type jsonArrayScanner struct {
	d *json.Decoder
	r io.ReadCloser
}

func (js *jsonArrayScanner) NextBatch(ctx context.Context) (service.MessageBatch, error) {
	if js.r == nil {
		return nil, io.EOF
	}

	jsonDocObj, err := readNextElement(js.d)
	if err != nil {
		_ = js.r.Close()
		js.r = nil
		return nil, err
	}

	msg := service.NewMessage(nil)
	msg.SetStructuredMut(jsonDocObj)

	return service.MessageBatch{msg}, nil
}

func (js *jsonArrayScanner) Close(ctx context.Context) error {
	if js.r == nil {
		return nil
	}
	return js.r.Close()
}
