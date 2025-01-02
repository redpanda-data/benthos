package stream_builder_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	_ "github.com/redpanda-data/benthos/v4/public/components/pure"
	"github.com/redpanda-data/benthos/v4/public/service"
)

type foobarInput struct {
	done       bool
	ackErrChan chan error
}

func newFoobarInput() *foobarInput {
	return &foobarInput{
		ackErrChan: make(chan error),
	}
}

func (i *foobarInput) Connect(context.Context) error {
	return nil
}

func (i *foobarInput) Read(context.Context) (*service.Message, service.AckFunc, error) {
	if i.done {
		return nil, nil, service.ErrEndOfInput
	}
	i.done = true

	return service.NewMessage([]byte("foobar")), func(_ context.Context, err error) error {
		i.ackErrChan <- err

		return nil
	}, nil
}

func (i *foobarInput) Close(context.Context) error {
	close(i.ackErrChan)

	return nil
}

func TestStreamDefaultOutput(t *testing.T) {
	input := newFoobarInput()

	// This test sits in its own package so it will get a fresh `service.GlobalEnvironment()` that we can alter safely.
	err := service.RegisterInput("foobar", service.NewConfigSpec(),
		func(_ *service.ParsedConfig, mgr *service.Resources) (service.Input, error) {
			return input, nil
		},
	)
	require.NoError(t, err)

	// We only imported the pure components so `reject` is selected as the default output instead of `stdout`.
	_, ok := service.GlobalEnvironment().GetOutputConfig("stdout")
	require.False(t, ok, "stdout output registered")

	streamBuilder := service.NewStreamBuilder()
	require.NoError(t, streamBuilder.SetYAML(`
input:
  foobar: {}
`))
	require.NoError(t, streamBuilder.SetLoggerYAML("level: OFF"))

	stream, err := streamBuilder.Build()
	require.NoError(t, err)

	closeChan := make(chan struct{})
	go func() {
		require.NoError(t, stream.Run(context.Background()))
		close(closeChan)
	}()

	select {
	case err := <-input.ackErrChan:
		require.ErrorContains(t, err, "message rejected by default because an output is not configured")
	case <-time.After(1 * time.Second):
		require.Fail(t, "timeout waiting for ack error")
	}

	// Wait for stream to shutdown automatically.
	select {
	case <-closeChan:
	case <-time.After(1 * time.Second):
		require.Fail(t, "timeout waiting for ack error")
	}
}
