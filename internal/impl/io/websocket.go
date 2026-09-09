// Copyright 2026 Redpanda Data, Inc.

package io

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
)

// checkTLSScheme rejects a config that enables TLS against a ws:// URL.
//
// Gorilla selects the transport from the URL scheme alone and applies
// TLSClientConfig only for wss. With ws:// the TLS settings are silently ignored,
// the connection is plaintext, and any configured auth header goes out in the clear.
// Fail at construction so the contradiction is visible instead of silent.
func checkTLSScheme(u *url.URL, tlsEnabled bool) error {
	if tlsEnabled && u.Scheme == "ws" {
		return fmt.Errorf("tls is enabled but url scheme is ws, use wss:// for %v", u.Redacted())
	}
	return nil
}

// hiddenDeadlineContext strips ctx.Deadline() while preserving cancellation.
//
// Gorilla sets a socket deadline matching ctx.Deadline(). Because the runtime's
// socket deadline timer and Go's context timer run independently, a socket read
// can time out (returning a generic "i/o timeout") slightly before ctx.Err() is set.
//
// Concealing the deadline prevents Gorilla from setting this competing socket timer.
// This ensures our context.AfterFunc watcher exclusively controls socket interruption
// and deterministically returns context errors.
type hiddenDeadlineContext struct {
	context.Context
}

func (hiddenDeadlineContext) Deadline() (time.Time, bool) {
	return time.Time{}, false
}

// dialContext is dialer.DialContext plus cancellation of the HTTP upgrade exchange.
//
// Gorilla applies the context to the TCP and TLS handshakes only.
// It then performs the websocket upgrade on a bare connection, which does not respond to context cancellation.
// A peer that accepts TCP and then stays silent blocks the dial until the handshake timeout.
func dialContext(ctx context.Context, dialer websocket.Dialer, urlStr string, headers http.Header) (*websocket.Conn, *http.Response, error) {
	netDialer := &net.Dialer{}
	var stop func() bool

	// 1. Intercept network connection creation to attach a context watcher (context.AfterFunc) that closes the conn once ctx is done.
	dialer.NetDialContext = func(dialCtx context.Context, network, addr string) (net.Conn, error) {
		conn, err := netDialer.DialContext(dialCtx, network, addr)
		if err != nil {
			return nil, err
		}
		// Watch ctx, not dialCtx: gorilla wraps dialCtx with HandshakeTimeout and
		// cancels it as DialContext returns, which would fire this on success.
		stop = context.AfterFunc(ctx, func() { _ = conn.Close() })
		return conn, nil
	}

	// 2. Perform dial with hidden deadline context.
	// The context we give gorilla keeps the cancellation of ctx (stopping TCP/TLS
	// handshakes), but conceals ctx.Deadline() so gorilla does not set a competing
	// socket timer that could race with ctx cancellation.
	client, res, err := dialer.DialContext(hiddenDeadlineContext{ctx}, urlStr, headers)

	// 3. Clean up and prioritize context error returns.
	if stop != nil {
		_ = stop()
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		if client != nil {
			_ = client.Close()
		}
		return nil, res, ctxErr
	}
	return client, res, err
}
