// Copyright 2025 Redpanda Data, Inc.

package io

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/internal/component"
	"github.com/redpanda-data/benthos/v4/internal/manager/mock"
	"github.com/redpanda-data/benthos/v4/internal/message"
)

func TestWebsocketBasic(t *testing.T) {
	expMsgs := []string{
		"foo",
		"bar",
		"baz",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}

		var ws *websocket.Conn
		var err error
		if ws, err = upgrader.Upgrade(w, r, nil); err != nil {
			return
		}

		defer ws.Close()

		for _, msg := range expMsgs {
			if err = ws.WriteMessage(websocket.BinaryMessage, []byte(msg)); err != nil {
				t.Error(err)
			}
		}
	}))

	wsURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	wsURL.Scheme = "ws"

	pConf, err := websocketInputSpec().ParseYAML(fmt.Sprintf(`
url: %v
`, wsURL.String()), nil)
	require.NoError(t, err)

	m, err := newWebsocketReaderFromParsed(pConf, mock.NewManager())
	require.NoError(t, err)

	ctx := t.Context()

	if err = m.Connect(ctx); err != nil {
		t.Fatal(err)
	}

	for _, exp := range expMsgs {
		var actMsg message.Batch
		if actMsg, _, err = m.ReadBatch(ctx); err != nil {
			t.Error(err)
		} else if act := string(actMsg.Get(0).AsBytes()); act != exp {
			t.Errorf("Wrong result: %v != %v", act, exp)
		}
	}

	require.NoError(t, m.Close(ctx))
}

func TestWebsocketOpenMsg(t *testing.T) {
	expMsgs := []string{
		"foo",
		"bar",
		"baz",
	}

	testHandler := func(expMsgType int, w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}

		var ws *websocket.Conn
		var err error
		if ws, err = upgrader.Upgrade(w, r, nil); err != nil {
			return
		}

		defer ws.Close()

		msgType, data, err := ws.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		if exp, act := "hello world", string(data); exp != act {
			t.Errorf("Wrong open message: %v != %v", act, exp)
		}
		if msgType != expMsgType {
			t.Errorf("Wrong open message type: %v != %v", msgType, expMsgType)
		}

		for _, msg := range expMsgs {
			if err = ws.WriteMessage(websocket.BinaryMessage, []byte(msg)); err != nil {
				t.Error(err)
			}
		}
	}

	tests := []struct {
		handler       func(expMsgType int, w http.ResponseWriter, r *http.Request)
		openMsgType   wsOpenMsgType
		wsOpenMsgType int
		errStr        string
	}{
		{
			handler:       testHandler,
			openMsgType:   wsOpenMsgTypeBinary,
			wsOpenMsgType: websocket.BinaryMessage,
		},
		{
			handler:       testHandler,
			openMsgType:   wsOpenMsgTypeText,
			wsOpenMsgType: websocket.TextMessage,
		},
		{
			// Use a simplified handler to avoid the blocking call to `ws.ReadMessage()` when no OpenMsg gets sent
			handler: func(_ int, w http.ResponseWriter, r *http.Request) {
				upgrader := websocket.Upgrader{}

				var ws *websocket.Conn
				var err error
				if ws, err = upgrader.Upgrade(w, r, nil); err != nil {
					return
				}

				ws.Close()
			},
			openMsgType: "foobar",
			errStr:      "unrecognised open_message_type: foobar",
		},
	}

	for id, test := range tests {
		t.Run(strconv.Itoa(id), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { test.handler(test.wsOpenMsgType, w, r) }))
			t.Cleanup(server.Close)

			wsURL, err := url.Parse(server.URL)
			require.NoError(t, err)

			wsURL.Scheme = "ws"

			pConf, err := websocketInputSpec().ParseYAML(fmt.Sprintf(`
url: %v
open_message: "hello world"
open_message_type: %v
`, wsURL.String(), test.openMsgType), nil)
			require.NoError(t, err)

			m, err := newWebsocketReaderFromParsed(pConf, mock.NewManager())
			require.NoError(t, err)

			ctx, done := context.WithTimeout(t.Context(), 100*time.Millisecond)
			t.Cleanup(func() { require.NoError(t, m.Close(ctx)) })
			t.Cleanup(done)

			if err = m.Connect(ctx); err != nil {
				if test.errStr != "" {
					require.ErrorContains(t, err, test.errStr)
					return
				}

				t.Fatal(err)
			}

			for _, exp := range expMsgs {
				var actMsg message.Batch
				if actMsg, _, err = m.ReadBatch(ctx); err != nil {
					t.Error(err)
				} else if act := string(actMsg.Get(0).AsBytes()); act != exp {
					t.Errorf("Wrong result: %v != %v", act, exp)
				}
			}

			require.NoError(t, m.Close(ctx))
		})
	}
}

func TestWebsocketMaxMessageSize(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}

		var ws *websocket.Conn
		var err error
		if ws, err = upgrader.Upgrade(w, r, nil); err != nil {
			return
		}

		defer ws.Close()

		if err = ws.WriteMessage(websocket.BinaryMessage, []byte("small")); err != nil {
			t.Error(err)
		}
		if err = ws.WriteMessage(websocket.BinaryMessage, bytes.Repeat([]byte("x"), 1024)); err != nil {
			t.Error(err)
		}

		// Wait for the client to drop the connection.
		_, _, _ = ws.ReadMessage()
	}))
	t.Cleanup(server.Close)

	wsURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	wsURL.Scheme = "ws"

	pConf, err := websocketInputSpec().ParseYAML(fmt.Sprintf(`
url: %v
max_message_size: 100
`, wsURL.String()), nil)
	require.NoError(t, err)

	m, err := newWebsocketReaderFromParsed(pConf, mock.NewManager())
	require.NoError(t, err)

	ctx := t.Context()

	require.NoError(t, m.Connect(ctx))

	actMsg, _, err := m.ReadBatch(ctx)
	require.NoError(t, err)
	require.Equal(t, "small", string(actMsg.Get(0).AsBytes()))

	_, _, err = m.ReadBatch(ctx)
	require.ErrorIs(t, err, component.ErrNotConnected)

	require.NoError(t, m.Close(ctx))
}

func TestWebsocketMaxMessageSizeDefault(t *testing.T) {
	pConf, err := websocketInputSpec().ParseYAML(`
url: ws://localhost:4195/get/ws
`, nil)
	require.NoError(t, err)

	m, err := newWebsocketReaderFromParsed(pConf, mock.NewManager())
	require.NoError(t, err)
	require.Equal(t, defaultMaxMessageSize, m.maxMsgSize)
}

func TestWebsocketMaxMessageSizeNegative(t *testing.T) {
	pConf, err := websocketInputSpec().ParseYAML(`
url: ws://localhost:4195/get/ws
max_message_size: -1
`, nil)
	require.NoError(t, err)

	_, err = newWebsocketReaderFromParsed(pConf, mock.NewManager())
	require.ErrorContains(t, err, "max_message_size must not be negative")
}

func TestWebsocketClose(t *testing.T) {
	closeChan := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}

		var ws *websocket.Conn
		var err error
		if ws, err = upgrader.Upgrade(w, r, nil); err != nil {
			return
		}

		defer ws.Close()
		<-closeChan
	}))

	wsURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	wsURL.Scheme = "ws"

	pConf, err := websocketInputSpec().ParseYAML(fmt.Sprintf(`
url: %v
`, wsURL.String()), nil)
	require.NoError(t, err)

	m, err := newWebsocketReaderFromParsed(pConf, mock.NewManager())
	require.NoError(t, err)

	ctx := t.Context()

	if err = m.Connect(ctx); err != nil {
		t.Fatal(err)
	}

	wg := sync.WaitGroup{}
	wg.Go(func() {
		require.NoError(t, m.Close(ctx))
	})

	if _, _, err = m.ReadBatch(ctx); err != component.ErrTypeClosed && err != component.ErrNotConnected {
		t.Errorf("Wrong error: %v != %v", err, component.ErrTypeClosed)
	}

	wg.Wait()
	close(closeChan)
}

// newWebsocketReader returns a websocketReader configured to connect to the websocket server listening on addr.
func newWebsocketReader(t *testing.T, addr net.Addr) *websocketReader {
	t.Helper()

	pConf, err := websocketInputSpec().ParseYAML("url: ws://"+addr.String()+"\n", nil)
	require.NoError(t, err)

	m, err := newWebsocketReaderFromParsed(pConf, mock.NewManager())
	require.NoError(t, err)

	return m
}

// newHangingWebsocketReader returns a reader pointed at a listener that accepts
// TCP connections but never answers the handshake, along with the accept channel
// of that listener.
func newHangingWebsocketReader(t *testing.T) (*websocketReader, <-chan struct{}) {
	t.Helper()

	addr, accepted := newHangingListener(t)
	return newWebsocketReader(t, addr), accepted
}

// newUnreachableWebsocketReader returns a reader pointed at a closed port. A dial
// there fails immediately, so a context error proves no dial was attempted.
func newUnreachableWebsocketReader(t *testing.T) *websocketReader {
	t.Helper()

	return newWebsocketReader(t, newUnreachableAddr(t))
}

// newIdleWebsocketReader returns a reader pointed at a server that completes the
// handshake and then sends nothing, so only the context can unblock a read.
func newIdleWebsocketReader(t *testing.T) *websocketReader {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()

		// Hold the connection open until the client goes away.
		for {
			if _, _, err := ws.ReadMessage(); err != nil {
				return
			}
		}
	}))
	t.Cleanup(server.Close)

	return newWebsocketReader(t, server.Listener.Addr())
}

// TestWebsocketConnectContextDone tests that Connect reports the context error
// when the context is done before or during the websocket handshake.
func TestWebsocketConnectContextDone(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// setUp returns a reader and a context that is already done, or that becomes
		// done while the dial waits for the handshake response. It also returns the
		// accept channel to check after Connect returns, or nil for a case where no
		// accept is expected.
		setUp   func(*testing.T) (*websocketReader, context.Context, <-chan struct{})
		wantErr error
	}{
		{
			name: "canceled before dialing",
			setUp: func(t *testing.T) (*websocketReader, context.Context, <-chan struct{}) {
				// The port is closed, so a dial fails at once. Only a context check
				// before the dial can give context.Canceled here.
				ctx, cancel := context.WithCancel(t.Context())
				cancel()
				return newUnreachableWebsocketReader(t), ctx, nil
			},
			wantErr: context.Canceled,
		},
		{
			name: "canceled while dialing",
			setUp: func(t *testing.T) (*websocketReader, context.Context, <-chan struct{}) {
				m, accepted := newHangingWebsocketReader(t)

				ctx, cancel := context.WithCancel(t.Context())
				t.Cleanup(cancel)
				// Cancel from the accept, not from a timer, so the dial always waits
				// for the handshake response when the context becomes done.
				go func() {
					<-accepted
					cancel()
				}()

				return m, ctx, nil
			},
			wantErr: context.Canceled,
		},
		{
			name: "deadline exceeded while dialing",
			setUp: func(t *testing.T) (*websocketReader, context.Context, <-chan struct{}) {
				m, accepted := newHangingWebsocketReader(t)

				// A context carries its deadline from the start, so this case cannot
				// take its trigger from the accept. The accept check after Connect
				// returns proves the deadline expired during the handshake.
				ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
				t.Cleanup(cancel)

				return m, ctx, accepted
			},
			wantErr: context.DeadlineExceeded,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			m, ctx, accepted := test.setUp(t)

			err := awaitWithin(t, 500*time.Millisecond, "Connect", func() error {
				return m.Connect(ctx)
			})
			require.ErrorIs(t, err, test.wantErr)

			if accepted != nil {
				requireAccepted(t, 5*time.Second, accepted)
			}
		})
	}
}

// TestWebsocketReadBatchContextDone tests that ReadBatch reports the context error instead of staying blocked on the
// socket read of an established connection.
func TestWebsocketReadBatchContextDone(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		newCtx  func(*testing.T) context.Context
		wantErr error
	}{
		{
			name: "canceled before reading",
			newCtx: func(t *testing.T) context.Context {
				ctx, cancel := context.WithCancel(t.Context())
				cancel()
				return ctx
			},
			wantErr: context.Canceled,
		},
		{
			name: "canceled while reading",
			newCtx: func(t *testing.T) context.Context {
				ctx, cancel := context.WithCancel(t.Context())
				t.Cleanup(cancel)
				// ReadBatch returns ctx.Err() whether the context goes done before
				// or during the read, so a small delay cannot make this case flake.
				time.AfterFunc(5*time.Millisecond, cancel)
				return ctx
			},
			wantErr: context.Canceled,
		},
		{
			name: "deadline exceeded while reading",
			newCtx: func(t *testing.T) context.Context {
				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Millisecond)
				t.Cleanup(cancel)
				return ctx
			},
			wantErr: context.DeadlineExceeded,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			m := newIdleWebsocketReader(t)
			require.NoError(t, m.Connect(t.Context()))
			// t.Context is canceled before cleanups run, so close with our own.
			t.Cleanup(func() { _ = m.Close(context.Background()) })

			ctx := test.newCtx(t)
			err := awaitWithin(t, 500*time.Millisecond, "ReadBatch", func() error {
				_, _, err := m.ReadBatch(ctx)
				return err
			})
			require.ErrorIs(t, err, test.wantErr)
		})
	}
}

// TestWebsocketReadBatchCancelKeepsMessage tests that ReadBatch still delivers
// an already-received message even if the context is done by then.
// A websocket server never redelivers a message once sent.
//
// Cancellation closes the connection, and the close must not discard an already
// buffered message. To pin that deterministically, the server sends two messages
// in a single TCP write. The client's socket read then buffers both frames
// together in userspace. The second ReadBatch parses its message from that buffer
// and must return it even if the context was cancelled.
//
// Note on scope and limitations:
// This test asserts the unit-level contract of websocketReader in isolation.
// In practice, consumer components driving this reader (such as AsyncReader)
// will NOT attempt further reads once context cancellation / soft-stop is
// signalled, and may discard batches returned during teardown. Full end-to-end
// in-flight message preservation during graceful shutdown remains an open gap
// requiring framework-level drain support.
func TestWebsocketReadBatchCancelKeepsMessage(t *testing.T) {
	first, second := []byte("read first"), []byte("won't be redelivered")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()

		// Craft the two frames by hand and send them in one write. Two
		// WriteMessage calls would be two writes, and the client's buffer
		// fill could then pick up the first frame without the second: two
		// separate writes often do land in the same read on loopback, but
		// nothing guarantees it, so the test forces both into one.
		var frames bytes.Buffer
		for _, msg := range [][]byte{first, second} {
			frames.WriteByte(0x80 | websocket.BinaryMessage) // FIN flag + opcode
			frames.WriteByte(byte(len(msg)))                 // unmasked, single length byte
			frames.Write(msg)
		}
		if _, err := ws.NetConn().Write(frames.Bytes()); err != nil {
			t.Error(err)
			return
		}

		// Hold the connection open until the client goes away.
		for {
			if _, _, err := ws.ReadMessage(); err != nil {
				return
			}
		}
	}))
	t.Cleanup(server.Close)

	// Init the websocket reader
	m := newWebsocketReader(t, server.Listener.Addr())
	require.NoError(t, m.Connect(t.Context()))
	t.Cleanup(func() { _ = m.Close(context.Background()) })

	// Read the first message
	batch, _, err := m.ReadBatch(t.Context())
	require.NoError(t, err)
	require.Equal(t, first, batch.Get(0).AsBytes())

	// Cancel the context (simulate a shutdown)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	// The context is already done, but the second frame is already buffered from
	// the earlier socket read, so ReadBatch must return it instead of an error.
	batch, _, err = m.ReadBatch(ctx)
	require.NoError(t, err)
	require.Equal(t, second, batch.Get(0).AsBytes())

	// The cancellation closed the connection, so the next read reports a lost
	// connection for the reconnect path.
	_, _, err = m.ReadBatch(t.Context())
	require.ErrorIs(t, err, component.ErrNotConnected)
}
