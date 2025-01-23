// Copyright 2025 Redpanda Data, Inc.

package service

import (
	"context"
	"errors"
	"sync"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/Jeffail/shutdown"

	"github.com/redpanda-data/benthos/v4/internal/api"
	"github.com/redpanda-data/benthos/v4/internal/cli/common"
	"github.com/redpanda-data/benthos/v4/internal/component/metrics"
	"github.com/redpanda-data/benthos/v4/internal/log"
	"github.com/redpanda-data/benthos/v4/internal/manager"
	"github.com/redpanda-data/benthos/v4/internal/stream"
)

// Stream executes a full Benthos stream and provides methods for performing
// status checks, terminating the stream, and blocking until the stream ends.
type Stream struct {
	strm    *stream.Type
	httpAPI *api.Type
	strmMut sync.Mutex
	shutSig *shutdown.Signaller
	onStart func()

	conf   stream.Config
	mgr    *manager.Type
	stats  metrics.Type
	tracer trace.TracerProvider
	logger log.Modular
}

func newStream(
	conf stream.Config,
	httpAPI *api.Type,
	mgr *manager.Type,
	stats metrics.Type,
	tracer trace.TracerProvider,
	logger log.Modular,
	onStart func(),
) *Stream {
	return &Stream{
		conf:    conf,
		httpAPI: httpAPI,
		mgr:     mgr,
		stats:   stats,
		tracer:  tracer,
		logger:  logger,
		shutSig: shutdown.NewSignaller(),
		onStart: onStart,
	}
}

// Resources returns a pointer to the common resources type of the stream.
func (s *Stream) Resources() *Resources {
	return newResourcesFromManager(s.mgr)
}

// Run attempts to start the stream pipeline and blocks until either the stream
// has gracefully come to a stop, or the provided context is cancelled.
func (s *Stream) Run(ctx context.Context) (err error) {
	s.strmMut.Lock()
	if s.strm != nil {
		err = errors.New("stream has already been run")
	} else {
		s.strm, err = stream.New(s.conf, s.mgr,
			stream.OptOnClose(func() {
				s.shutSig.TriggerHasStopped()
			}))
	}
	s.strmMut.Unlock()
	if err != nil {
		return
	}

	if s.httpAPI != nil {
		go func() {
			_ = s.httpAPI.ListenAndServe()
		}()
	}
	go s.onStart()

	select {
	case <-s.shutSig.HasStoppedChan():
		return s.Stop(ctx)
	case <-ctx.Done():
	}
	return ctx.Err()
}

// StopWithin attempts to close the stream within the specified timeout period.
// Initially the attempt is graceful, but as the timeout draws close the attempt
// becomes progressively less graceful.
//
// An ungraceful shutdown increases the likelihood of processing duplicate
// messages on the next start up, but never results in dropped messages as long
// as the input source supports at-least-once delivery.
func (s *Stream) StopWithin(timeout time.Duration) error {
	ctx, done := context.WithTimeout(context.Background(), timeout)
	defer done()
	return s.Stop(ctx)
}

// Stop attempts to close the stream gracefully, but if the context is closed or
// draws near to a deadline the attempt becomes less graceful.
//
// An ungraceful shutdown increases the likelihood of processing duplicate
// messages on the next start up, but never results in dropped messages as long
// as the input source supports at-least-once delivery.
func (s *Stream) Stop(ctx context.Context) (err error) {
	s.strmMut.Lock()
	strm := s.strm
	s.strmMut.Unlock()
	if strm == nil {
		return errors.New("stream has not been run yet")
	}

	stopStats := s.stats
	closeStats := func() error {
		if stopStats == nil {
			return nil
		}
		err := stopStats.Close()
		stopStats = nil
		return err
	}

	stopTracer := s.tracer
	closeTracer := func(ctx context.Context) error {
		if stopTracer == nil {
			return nil
		}
		if shutter, ok := stopTracer.(interface {
			Shutdown(context.Context) error
		}); ok {
			return shutter.Shutdown(ctx)
		}
		return nil
	}

	stopHTTP := s.httpAPI
	closeHTTP := func(ctx context.Context) error {
		if stopHTTP == nil {
			return nil
		}
		err := s.httpAPI.Shutdown(ctx)
		stopHTTP = nil
		return err
	}

	defer func() {
		if err == nil {
			return
		}

		// Still attempt to shut down other resources on an error, but do not
		// block.
		s.mgr.TriggerStopConsuming()
		_ = closeStats()
		_ = closeTracer(context.Background())
		_ = closeHTTP(context.Background())
	}()

	if err = strm.Stop(ctx); err != nil {
		return
	}

	s.mgr.TriggerStopConsuming()
	if err = s.mgr.WaitForClose(ctx); err != nil {
		return
	}

	if err = closeStats(); err != nil {
		return
	}

	if err = closeTracer(ctx); err != nil {
		return
	}

	err = closeHTTP(ctx)
	return
}

//------------------------------------------------------------------------------

// RunningStreamSummary represents a running stream and provides access to
// information such as the connectivity status of the plugins running within.
type RunningStreamSummary struct {
	c common.RunningStream
}

// ConnectionStatus represents a current plugin component connection. The
// component can be identified by the label and/or the path of the component as
// found in a parsed config.
type ConnectionStatus struct {
	label     string
	path      []string
	connected bool
	err       error
}

// Label returns the label of the component, or an empty string if omitted.
func (c ConnectionStatus) Label() string {
	return c.label
}

// Path returns the path of the component as found in a parsed config.
func (c ConnectionStatus) Path() []string {
	return c.path
}

// Active returns true if the connection is currently active.
func (c ConnectionStatus) Active() bool {
	return c.connected
}

// Err returns an error preventing the connection when appropriate. An inactive
// connection may still yield a nil error in cases where the connection has not
// yet been attempted (during initialisation) or if the connection was
// intentionally closed (during shutdown).
func (c ConnectionStatus) Err() error {
	return c.err
}

// ConnectionStatuses returns a list of connection statuses, one for each
// currently active plugin component. Not all components will yield a
// connectivity status, this is true for all broker types and orchestration
// components, but the child components they manage will yield where possible.
func (r *RunningStreamSummary) ConnectionStatuses() []ConnectionStatus {
	statuses := r.c.ConnectionStatus()
	conns := make([]ConnectionStatus, 0, len(statuses))
	for _, s := range statuses {
		conns = append(conns, ConnectionStatus{
			label:     s.Label,
			path:      s.Path,
			connected: s.Connected,
			err:       s.Err,
		})
	}
	return conns
}
