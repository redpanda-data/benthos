// Copyright 2025 Redpanda Data, Inc.

package io

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/redpanda-data/benthos/v4/internal/component"
	"github.com/redpanda-data/benthos/v4/public/bloblang"
	"github.com/redpanda-data/benthos/v4/public/service"
	"github.com/redpanda-data/benthos/v4/public/service/codec"
)

const (
	isFieldNetwork            = "network"
	isFieldAddress            = "address"
	isFieldOpenMessageMapping = "open_message_mapping"
)

func socketInputSpec() *service.ConfigSpec {
	return service.NewConfigSpec().
		Stable().
		Summary(`Connects to a tcp or unix socket and consumes a continuous stream of messages.`).
		Categories("Network").
		Fields(
			service.NewStringEnumField(isFieldNetwork, "unix", "tcp").
				Description("A network type to assume (unix|tcp)."),
			service.NewStringField(isFieldAddress).
				Description("The address to connect to.").
				Examples("/tmp/benthos.sock", "127.0.0.1:6000"),
			service.NewAutoRetryNacksToggleField(),
			service.NewBloblangField(isFieldOpenMessageMapping).
				Description("An optional xref:guides:bloblang/about.adoc[Bloblang mapping] which should evaluate to a string which will be sent upstream before the downstream data flow starts.").
				Example(`root = "username,password"`).
				Optional(),
		).
		Fields(codec.DeprecatedCodecFields("lines")...)
}

func init() {
	service.MustRegisterBatchInput("socket", socketInputSpec(), func(conf *service.ParsedConfig, mgr *service.Resources) (service.BatchInput, error) {
		i, err := newSocketReaderFromParsed(conf, mgr)
		if err != nil {
			return nil, err
		}
		// TODO: Inject async cut off?
		return service.AutoRetryNacksBatchedToggled(conf, i)
	})
}

type socketReader struct {
	log *service.Logger

	address            string
	network            string
	codecCtor          codec.DeprecatedFallbackCodec
	openMessageMapping *bloblang.Executor

	codecMut sync.Mutex
	codec    codec.DeprecatedFallbackStream
}

func newSocketReaderFromParsed(pConf *service.ParsedConfig, mgr *service.Resources) (rdr *socketReader, err error) {
	rdr = &socketReader{
		log: mgr.Logger(),
	}
	if rdr.address, err = pConf.FieldString(isFieldAddress); err != nil {
		return
	}
	if rdr.network, err = pConf.FieldString(isFieldNetwork); err != nil {
		return
	}
	if rdr.codecCtor, err = codec.DeprecatedCodecFromParsed(pConf); err != nil {
		return
	}
	if pConf.Contains(isFieldOpenMessageMapping) {
		if rdr.openMessageMapping, err = pConf.FieldBloblang(isFieldOpenMessageMapping); err != nil {
			return nil, err
		}
	}
	return
}

func (s *socketReader) Connect(ctx context.Context) error {
	s.codecMut.Lock()
	defer s.codecMut.Unlock()

	if s.codec != nil {
		return nil
	}

	conn, err := net.Dial(s.network, s.address)
	if err != nil {
		return err
	}

	if s.codec, err = s.codecCtor.Create(conn, func(ctx context.Context, err error) error {
		return nil
	}, service.NewScannerSourceDetails()); err != nil {
		conn.Close()
		return err
	}

	if s.openMessageMapping != nil {
		var openMessage string
		if queryResult, err := s.openMessageMapping.Query(nil); err != nil {
			return fmt.Errorf("open message mapping failed: %s", err)
		} else {
			var ok bool
			if openMessage, ok = queryResult.(string); !ok {
				return fmt.Errorf("open message mapping returned non-string result: %T", queryResult)
			}

			if openMessage == "" {
				return errors.New("open message mapping returned empty string")
			}
		}

		if _, err := conn.Write([]byte(openMessage)); err != nil {
			return fmt.Errorf("failed to write open message: %s", err)
		}
	}

	return nil
}

func (s *socketReader) ReadBatch(ctx context.Context) (service.MessageBatch, service.AckFunc, error) {
	s.codecMut.Lock()
	codec := s.codec
	s.codecMut.Unlock()

	if codec == nil {
		return nil, nil, service.ErrNotConnected
	}

	parts, codecAckFn, err := codec.NextBatch(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) ||
			errors.Is(err, context.DeadlineExceeded) {
			err = component.ErrTimeout
		}
		if err != component.ErrTimeout {
			s.codecMut.Lock()
			if s.codec != nil && s.codec == codec {
				s.codec.Close(ctx)
				s.codec = nil
			}
			s.codecMut.Unlock()
		}
		if errors.Is(err, io.EOF) {
			return nil, nil, component.ErrTimeout
		}
		return nil, nil, err
	}

	// We simply bounce rejected messages in a loop downstream so there's no
	// benefit to aggregating acks.
	_ = codecAckFn(context.Background(), nil)

	if len(parts) == 0 {
		return nil, nil, component.ErrTimeout
	}

	return parts, func(rctx context.Context, res error) error {
		return nil
	}, nil
}

func (s *socketReader) Close(ctx context.Context) (err error) {
	s.codecMut.Lock()
	defer s.codecMut.Unlock()

	if s.codec != nil {
		err = s.codec.Close(ctx)
		s.codec = nil
	}

	return
}
