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
