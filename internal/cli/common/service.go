// Copyright 2025 Redpanda Data, Inc.

package common

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/redpanda-data/benthos/v4/internal/config"
	"github.com/redpanda-data/benthos/v4/internal/manager"
	"github.com/redpanda-data/benthos/v4/internal/stream"
	strmmgr "github.com/redpanda-data/benthos/v4/internal/stream/manager"

	"github.com/urfave/cli/v2"
)

// ErrExitCode is an error that could be returned by the cli application in
// order to indicate that a given exit code should be returned by the process.
type ErrExitCode struct {
	Err  error
	Code int
}

// Error returns the underlying error string.
func (e *ErrExitCode) Error() string {
	return e.Err.Error()
}

// Unwrap returns the underlying error.
func (e *ErrExitCode) Unwrap() error {
	return e.Err
}

// RunService runs a service command (either the default or the streams
// subcommand).
func RunService(c *cli.Context, cliOpts *CLIOpts, streamsMode bool) error {
	if err := cliOpts.CustomRunExtractFn(c); err != nil {
		return err
	}

	mainPath, inferredMainPath, confReader := ReadConfig(c, cliOpts, streamsMode)

	conf, pConf, lints, err := confReader.Read()
	if err != nil {
		return fmt.Errorf("configuration file read error: %w", err)
	}
	defer func() {
		_ = confReader.Close(c.Context)
	}()

	logger, err := CreateLogger(c, cliOpts, conf, streamsMode)
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}

	verLogger := logger.With("benthos_version", cliOpts.Version)
	if mainPath == "" {
		verLogger.Info("Running without a main config file")
	} else if inferredMainPath {
		verLogger.With("path", mainPath).Info("Running main config from file found in a default path")
	} else {
		verLogger.With("path", mainPath).Info("Running main config from specified file")
	}

	strict := !cliOpts.RootFlags.GetChilled(c)
	for _, lint := range lints {
		if strict {
			logger.With("lint", lint).Error("Config lint error")
		} else {
			logger.With("lint", lint).Warn("Config lint error")
		}
	}
	if strict && len(lints) > 0 {
		return errors.New(cliOpts.ExecTemplate("shutting down due to linter errors, to prevent shutdown run {{.ProductName}} with --chilled"))
	}

	stoppableManager, err := CreateManager(c, cliOpts, logger, streamsMode, conf)
	if err != nil {
		return err
	}

	if err := cliOpts.OnManagerInitialised(stoppableManager.mgr, pConf); err != nil {
		return err
	}

	var stoppableStream RunningStream
	var dataStreamClosedChan chan struct{}

	// Create data streams.
	watching := cliOpts.RootFlags.GetWatcher(c)
	if streamsMode {
		enableStreamsAPI := !c.Bool("no-api")
		stoppableStream, err = initStreamsMode(cliOpts, strict, watching, enableStreamsAPI, confReader, stoppableManager.Manager())
	} else {
		stoppableStream, dataStreamClosedChan, err = initNormalMode(cliOpts, conf, strict, watching, confReader, stoppableManager.Manager())
	}
	if err != nil {
		return err
	}

	if err := cliOpts.OnStreamInit(stoppableStream); err != nil {
		return err
	}
	return RunManagerUntilStopped(c, cliOpts, conf, stoppableManager, stoppableStream, dataStreamClosedChan)
}

// DelayShutdown attempts to block until either:
// - The delay period ends
// - The provided context is cancelled
// - The process receives an interrupt or sigterm
func DelayShutdown(ctx context.Context, duration time.Duration) error {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	delayCtx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()

	select {
	case <-delayCtx.Done():
		err := delayCtx.Err()
		if err != nil && err != context.DeadlineExceeded {
			return err
		}
	case sig := <-sigChan:
		return fmt.Errorf("shutdown delay interrupted by signal: %s", sig)
	}

	return nil
}

func initStreamsMode(
	opts *CLIOpts,
	strict, watching, enableAPI bool,
	confReader *config.Reader,
	mgr *manager.Type,
) (RunningStream, error) {
	logger := mgr.Logger()
	streamMgr := strmmgr.New(mgr, strmmgr.OptAPIEnabled(enableAPI))

	streamConfs := map[string]stream.Config{}
	lints, err := confReader.ReadStreams(streamConfs)
	if err != nil {
		return nil, fmt.Errorf("stream configuration file read error: %w", err)
	}

	for _, lint := range lints {
		if strict {
			logger.With("lint", lint).Error("Config lint error")
		} else {
			logger.With("lint", lint).Warn("Config lint error")
		}
	}
	if strict && len(lints) > 0 {
		return nil, errors.New("shutting down due to stream linter errors, to prevent shutdown run {{.ProductName}} with --chilled")
	}

	for id, conf := range streamConfs {
		if err := streamMgr.Create(id, conf); err != nil {
			return nil, fmt.Errorf("failed to create stream (%v): %w", id, err)
		}
	}
	logger.Info(opts.ExecTemplate("Launching {{.ProductName}} in streams mode, use CTRL+C to close"))

	if err := confReader.SubscribeStreamChanges(func(id string, newStreamConf *stream.Config) error {
		ctx, done := context.WithTimeout(context.Background(), time.Second*30)
		defer done()

		var updateErr error
		if newStreamConf != nil {
			if updateErr = streamMgr.Update(ctx, id, *newStreamConf); updateErr != nil && errors.Is(updateErr, strmmgr.ErrStreamDoesNotExist) {
				updateErr = streamMgr.Create(id, *newStreamConf)
			}
		} else {
			if updateErr = streamMgr.Delete(ctx, id); updateErr != nil && errors.Is(updateErr, strmmgr.ErrStreamDoesNotExist) {
				updateErr = nil
			}
		}
		return updateErr
	}); err != nil {
		return nil, fmt.Errorf("failed to create stream config watcher: %w", err)
	}

	if watching {
		if err := confReader.BeginFileWatching(mgr, strict); err != nil {
			return nil, fmt.Errorf("failed to create stream config watcher: %w", err)
		}
	}
	return streamMgr, nil
}

func initNormalMode(
	opts *CLIOpts,
	conf config.Type,
	strict, watching bool,
	confReader *config.Reader,
	mgr *manager.Type,
) (newStream RunningStream, stoppedChan chan struct{}, err error) {
	logger := mgr.Logger()

	stoppedChan = make(chan struct{})
	var closeOnce sync.Once
	streamInit := func() (RunningStream, error) {
		return stream.New(conf.Config, mgr, stream.OptOnClose(func() {
			if !watching {
				closeOnce.Do(func() {
					close(stoppedChan)
				})
			}
		}))
	}

	initStream, err := streamInit()
	if err != nil {
		return nil, nil, fmt.Errorf("service closing due to: %w", err)
	}

	stoppableStream := NewSwappableStopper(initStream)

	logger.Info(opts.ExecTemplate("Launching a {{.ProductName}} instance, use CTRL+C to close"))

	if err := confReader.SubscribeConfigChanges(func(newStreamConf *config.Type) error {
		ctx, done := context.WithTimeout(context.Background(), 30*time.Second)
		defer done()
		// NOTE: We're ignoring observability field changes for now.
		return stoppableStream.Replace(ctx, func() (RunningStream, error) {
			conf.Config = newStreamConf.Config
			return streamInit()
		})
	}); err != nil {
		return nil, nil, fmt.Errorf("failed to create config file watcher: %w", err)
	}

	if watching {
		if err := confReader.BeginFileWatching(mgr, strict); err != nil {
			return nil, nil, fmt.Errorf("failed to create config file watcher: %w", err)
		}
	}

	newStream = stoppableStream
	return
}
