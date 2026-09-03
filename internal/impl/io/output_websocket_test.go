// Copyright 2025 Redpanda Data, Inc.

package io

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/redpanda-data/benthos/v4/internal/manager/mock"
	"github.com/redpanda-data/benthos/v4/internal/message"
)

func TestWebsocketOutputBasic(t *testing.T) {
	ctx, done := context.WithTimeout(t.Context(), time.Second*30)
	defer done()

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

		var actBytes []byte
		for _, exp := range expMsgs {
			if _, actBytes, err = ws.ReadMessage(); err != nil {
				t.Error(err)
			} else if act := string(actBytes); act != exp {
				t.Errorf("Wrong msg contents: %v != %v", act, exp)
			}
		}
	}))

	wsURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	wsURL.Scheme = "ws"

	conf := parseYAMLOutputConf(t, `
websocket:
  url: %v
`, wsURL.String())

	m, err := mock.NewManager().NewOutput(conf)
	require.NoError(t, err)

	tChan := make(chan message.Transaction)
	require.NoError(t, m.Consume(tChan))

	m.TriggerStartConsuming()

	for _, msg := range expMsgs {
		require.NoError(t, writeBatchToChan(ctx, t, message.QuickBatch([][]byte{[]byte(msg)}), tChan))
	}

	m.TriggerCloseNow()
	require.NoError(t, m.WaitForClose(ctx))
}

func TestWebsocketOutputClose(t *testing.T) {
	ctx, done := context.WithTimeout(t.Context(), time.Second*30)
	defer done()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}

		var ws *websocket.Conn
		var err error
		if ws, err = upgrader.Upgrade(w, r, nil); err != nil {
			return
		}

		ws.Close()
	}))

	wsURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	wsURL.Scheme = "ws"

	conf := parseYAMLOutputConf(t, `
websocket:
  url: %v
`, wsURL.String())

	m, err := mock.NewManager().NewOutput(conf)
	require.NoError(t, err)

	tChan := make(chan message.Transaction)
	require.NoError(t, m.Consume(tChan))

	m.TriggerStartConsuming()

	m.TriggerCloseNow()
	require.NoError(t, m.WaitForClose(ctx))
}

// TestWebsocketOutputConnectContextDone tests that Connect reports the context
// error when the context is done before or during the websocket handshake.
func TestWebsocketOutputConnectContextDone(t *testing.T) {
	t.Parallel()

	newWebsocketWriter := func(t *testing.T, addr net.Addr) *websocketWriter {
		t.Helper()

		pConf, err := websocketOutputSpec().ParseYAML("url: ws://"+addr.String()+"\n", nil)
		require.NoError(t, err)

		w, err := newWebsocketWriterFromParsed(pConf, mock.NewManager())
		require.NoError(t, err)

		return w
	}

	newHangingWebsocketWriter := func(t *testing.T) (*websocketWriter, <-chan struct{}) {
		t.Helper()

		addr, accepted := newHangingListener(t)
		return newWebsocketWriter(t, addr), accepted
	}

	newUnreachableWebsocketWriter := func(t *testing.T) *websocketWriter {
		t.Helper()

		return newWebsocketWriter(t, newUnreachableAddr(t))
	}

	tests := []struct {
		name string
		// setUp returns a writer and a context that is already done, or that becomes
		// done while the dial waits for the handshake response. It also returns the
		// accept channel to check after Connect returns, or nil for a case where no
		// accept is expected.
		setUp   func(*testing.T) (*websocketWriter, context.Context, <-chan struct{})
		wantErr error
	}{
		{
			name: "canceled before dialing",
			setUp: func(t *testing.T) (*websocketWriter, context.Context, <-chan struct{}) {
				ctx, cancel := context.WithCancel(t.Context())
				cancel()
				// The port is closed, so a dial fails at once. Only a context check
				// before the dial can give context.Canceled here.
				return newUnreachableWebsocketWriter(t), ctx, nil
			},
			wantErr: context.Canceled,
		},
		{
			name: "canceled while dialing",
			setUp: func(t *testing.T) (*websocketWriter, context.Context, <-chan struct{}) {
				w, accepted := newHangingWebsocketWriter(t)

				ctx, cancel := context.WithCancel(t.Context())
				t.Cleanup(cancel)
				// Cancel from the accept, not from a timer, so the dial always waits
				// for the handshake response when the context becomes done.
				go func() {
					<-accepted
					cancel()
				}()

				return w, ctx, nil
			},
			wantErr: context.Canceled,
		},
		{
			name: "deadline exceeded while dialing",
			setUp: func(t *testing.T) (*websocketWriter, context.Context, <-chan struct{}) {
				w, accepted := newHangingWebsocketWriter(t)

				// A context carries its deadline from the start, so this case cannot
				// take its trigger from the accept. The accept check after Connect
				// returns proves the deadline expired during the handshake.
				ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
				t.Cleanup(cancel)

				return w, ctx, accepted
			},
			wantErr: context.DeadlineExceeded,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			w, ctx, accepted := test.setUp(t)

			err := awaitWithin(t, 500*time.Millisecond, "Connect", func() error {
				return w.Connect(ctx)
			})
			require.ErrorIs(t, err, test.wantErr)

			if accepted != nil {
				requireAccepted(t, 5*time.Second, accepted)
			}
		})
	}
}

// newTestWebsocketServer starts a websocket server that reads until the client closes the connection, then calls
// onClose (if non-nil). It registers its own cleanup.
func newTestWebsocketServer(t *testing.T, onClose func()) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()

		// Read until the client closes the connection.
		for {
			if _, _, err := ws.ReadMessage(); err != nil {
				break
			}
		}
		if onClose != nil {
			onClose()
		}
	}))
	t.Cleanup(server.Close)
	return server
}

// newConnectedTestWebsocketWriter builds a websocketWriter pointed at server and connects it. It registers its own
// cleanup.
func newConnectedTestWebsocketWriter(t *testing.T, server *httptest.Server) *websocketWriter {
	pConf, err := websocketOutputSpec().ParseYAML("url: ws://"+server.Listener.Addr().String()+"\n", nil)
	require.NoError(t, err)
	m, err := newWebsocketWriterFromParsed(pConf, mock.NewManager())
	require.NoError(t, err)
	t.Cleanup(func() { _ = m.Close(context.Background()) })

	require.NoError(t, m.Connect(t.Context()))
	return m
}

// TestWebsocketOutputWriteFailureClosesConn tests that a write failure on an otherwise healthy connection closes
// that connection before the writer forgets it. Without the close, the socket, its reader goroutine, and the
// server-side connection all leak on each failed write.
func TestWebsocketOutputWriteFailureClosesConn(t *testing.T) {
	// Set up a websocket server that tracks when the client closes the connection.
	serverSawClose := make(chan struct{})
	server := newTestWebsocketServer(t, func() { close(serverSawClose) })
	m := newConnectedTestWebsocketWriter(t, server)

	client := m.getWS()
	require.NotNil(t, client)

	// An expired write deadline makes the next write fail while the connection itself stays healthy.
	require.NoError(t, client.SetWriteDeadline(time.Unix(1, 0)))
	err := m.WriteBatch(t.Context(), message.QuickBatch([][]byte{[]byte("foo")}))
	require.Error(t, err)

	// Check that the failed connection is forgotten.
	require.Nil(t, m.getWS(), "the failed connection must be forgotten")

	// Check that the server saw the client close the connection.
	select {
	case <-serverSawClose:
	case <-time.After(2 * time.Second):
		t.Fatal("the server still holds the connection: the client did not close it")
	}
}

// TestWebsocketOutputDropConnIgnoresStaleConn checks the guard in dropConn: a call that carries a connection the
// writer already replaced must not clear the newer one. It calls dropConn directly, so it checks the contract of
// the guard, not the timing window in which WriteBatch would hit it.
func TestWebsocketOutputDropConnIgnoresStaleConn(t *testing.T) {
	server := newTestWebsocketServer(t, nil)
	m := newConnectedTestWebsocketWriter(t, server)

	// Establish a first connection, then replace it with a second one.
	stale := m.getWS()
	require.NotNil(t, stale)

	require.NoError(t, m.Close(t.Context()))
	require.NoError(t, m.Connect(t.Context()))
	current := m.getWS()
	require.NotNil(t, current)
	require.NotSame(t, stale, current)

	// A late drop of the stale connection must leave the current one in place and usable.
	m.dropConn(stale)
	require.Same(t, current, m.getWS(), "dropConn cleared a connection it was not given")
	require.NoError(t, m.WriteBatch(t.Context(), message.QuickBatch([][]byte{[]byte("foo")})))
}
