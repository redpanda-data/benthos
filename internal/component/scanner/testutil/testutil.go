// Copyright 2025 Redpanda Data, Inc.

package testutil

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/public/service"
)

type microReader struct {
	io.Reader
}

func (n microReader) Read(p []byte) (int, error) {
	// Only a max of 5 bytes at a time
	if len(p) < 5 {
		return n.Reader.Read(p)
	}

	micro := make([]byte, 5)
	byteCount, err := n.Reader.Read(micro)
	if err != nil {
		return byteCount, err
	}

	_ = copy(p, micro)
	return byteCount, nil
}

// ScannerTestSuite returns the test suite for scanners.
func ScannerTestSuite(t *testing.T, codec *service.OwnedScannerCreator, details *service.ScannerSourceDetails, data []byte, expected ...string) {
	if details == nil {
		details = &service.ScannerSourceDetails{}
	}

	t.Run("close before reading", func(t *testing.T) {
		buf := io.NopCloser(bytes.NewReader(data))

		ack := errors.New("default err")

		r, err := codec.Create(buf, func(ctx context.Context, err error) error {
			ack = err
			return nil
		}, details)
		require.NoError(t, err)

		assert.NoError(t, r.Close(t.Context()))
		assert.EqualError(t, ack, "service shutting down")
	})

	t.Run("can consume micro flushes", func(t *testing.T) {
		buf := io.NopCloser(microReader{bytes.NewReader(data)})

		ack := errors.New("default err")

		r, err := codec.Create(buf, func(ctx context.Context, err error) error {
			ack = err
			return nil
		}, details)
		require.NoError(t, err)

		allReads := map[string][]byte{}

		for _, exp := range expected {
			p, ackFn, err := r.NextBatch(t.Context())
			require.NoError(t, err)
			require.NoError(t, ackFn(t.Context(), nil))
			require.Len(t, p, 1)

			mBytes, err := p[0].AsBytes()
			require.NoError(t, err)
			assert.Equal(t, exp, string(mBytes))
			allReads[string(mBytes)] = mBytes
		}

		_, _, err = r.NextBatch(t.Context())
		assert.EqualError(t, err, "EOF")

		assert.NoError(t, r.Close(t.Context()))
		assert.NoError(t, ack)

		for k, v := range allReads {
			assert.Equal(t, k, string(v), "Must not corrupt previous reads")
		}
	})

	t.Run("acks ordered reads", func(t *testing.T) {
		buf := io.NopCloser(bytes.NewReader(data))

		ack := errors.New("default err")

		r, err := codec.Create(buf, func(ctx context.Context, err error) error {
			ack = err
			return nil
		}, details)
		require.NoError(t, err)

		allReads := map[string][]byte{}

		for _, exp := range expected {
			p, ackFn, err := r.NextBatch(t.Context())
			require.NoError(t, err)
			require.NoError(t, ackFn(t.Context(), nil))
			require.Len(t, p, 1)

			mBytes, err := p[0].AsBytes()
			require.NoError(t, err)
			assert.Equal(t, exp, string(mBytes))
			allReads[string(mBytes)] = mBytes
		}

		_, _, err = r.NextBatch(t.Context())
		assert.EqualError(t, err, "EOF")

		assert.NoError(t, r.Close(t.Context()))
		assert.NoError(t, ack)

		for k, v := range allReads {
			assert.Equal(t, k, string(v), "Must not corrupt previous reads")
		}
	})

	t.Run("acks unordered reads", func(t *testing.T) {
		buf := io.NopCloser(bytes.NewReader(data))

		ack := errors.New("default err")

		r, err := codec.Create(buf, func(ctx context.Context, err error) error {
			ack = err
			return nil
		}, details)
		require.NoError(t, err)

		allReads := map[string][]byte{}

		var ackFns []service.AckFunc
		for _, exp := range expected {
			p, ackFn, err := r.NextBatch(t.Context())
			require.NoError(t, err)
			require.Len(t, p, 1)
			ackFns = append(ackFns, ackFn)

			mBytes, err := p[0].AsBytes()
			require.NoError(t, err)
			assert.Equal(t, exp, string(mBytes))
			allReads[string(mBytes)] = mBytes
		}

		_, _, err = r.NextBatch(t.Context())
		assert.EqualError(t, err, "EOF")
		assert.NoError(t, r.Close(t.Context()))

		for _, ackFn := range ackFns {
			require.NoError(t, ackFn(t.Context(), nil))
		}

		assert.NoError(t, ack)

		for k, v := range allReads {
			assert.Equal(t, k, string(v), "Must not corrupt previous reads")
		}
	})

	t.Run("acks parallel reads", func(t *testing.T) {
		buf := io.NopCloser(bytes.NewReader(data))

		ack := errors.New("default err")

		r, err := codec.Create(buf, func(ctx context.Context, err error) error {
			ack = err
			return nil
		}, details)
		require.NoError(t, err)

		allReads := map[string][]byte{}

		wg := sync.WaitGroup{}
		wg.Add(len(expected))

		for _, exp := range expected {
			exp := exp
			p, ackFn, err := r.NextBatch(t.Context())
			require.NoError(t, err)
			require.Len(t, p, 1)

			mBytes, err := p[0].AsBytes()
			require.NoError(t, err)
			assert.Equal(t, exp, string(mBytes))
			allReads[string(mBytes)] = mBytes

			go func() {
				defer wg.Done()
				require.NoError(t, ackFn(t.Context(), nil))
			}()
		}

		_, _, err = r.NextBatch(t.Context())
		assert.EqualError(t, err, "EOF")

		wg.Wait()
		assert.NoError(t, r.Close(t.Context()))

		assert.NoError(t, ack)

		for k, v := range allReads {
			assert.Equal(t, k, string(v), "Must not corrupt previous reads")
		}
	})

	if len(expected) > 0 {
		t.Run("nacks unordered reads", func(t *testing.T) {
			buf := io.NopCloser(bytes.NewReader(data))

			ack := errors.New("default err")
			exp := errors.New("real err")

			r, err := codec.Create(buf, func(ctx context.Context, err error) error {
				ack = err
				return nil
			}, details)
			require.NoError(t, err)

			allReads := map[string][]byte{}

			var ackFns []service.AckFunc
			for _, exp := range expected {
				p, ackFn, err := r.NextBatch(t.Context())
				require.NoError(t, err)
				require.Len(t, p, 1)
				ackFns = append(ackFns, ackFn)

				mBytes, err := p[0].AsBytes()
				require.NoError(t, err)
				assert.Equal(t, exp, string(mBytes))
				allReads[string(mBytes)] = mBytes
			}

			_, _, err = r.NextBatch(t.Context())
			assert.EqualError(t, err, "EOF")
			assert.NoError(t, r.Close(t.Context()))

			for i, ackFn := range ackFns {
				if i == 0 {
					require.NoError(t, ackFn(t.Context(), exp))
				} else {
					require.NoError(t, ackFn(t.Context(), nil))
				}
			}

			assert.EqualError(t, ack, exp.Error())

			for k, v := range allReads {
				assert.Equal(t, k, string(v), "Must not corrupt previous reads")
			}
		})
	}
}
