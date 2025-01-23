// Copyright 2025 Redpanda Data, Inc.

//go:build wasm

package config

import (
	"errors"

	"github.com/redpanda-data/benthos/v4/internal/bundle"
)

// BeginFileWatching does nothing in WASM builds as it is not supported. Sorry!
func (r *Reader) BeginFileWatching(mgr bundle.NewManagement, strict bool) error {
	return errors.New("file watching is disabled in WASM builds")
}

// noReread is a no-op in WASM builds as the file watcher is not supported.
func noReread(err error) error {
	return err
}
