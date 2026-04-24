// Copyright 2026 Redpanda Data, Inc.

// Package bloblangv2 provides the public API for parsing and executing
// Bloblang V2 mappings, and for extending the language with user-defined
// methods and functions.
//
// Bloblang V2 is a redesigned mapping language shipped alongside the
// existing V1 implementation in public/bloblang. V2 and V1 are separate
// languages with separate parsers, interpreters, and plugin registries;
// plugins registered against one cannot be used from the other.
//
// The core types are:
//
//   - Environment: an isolated registry of methods and functions that
//     mappings may invoke. The global environment can be accessed via
//     GlobalEnvironment, or isolated ones built via NewEnvironment /
//     NewEmptyEnvironment.
//   - Executor: the compiled form of a mapping, produced by
//     Environment.Parse. Executors are safe for concurrent use.
//   - PluginSpec: a builder for declaring the signature of a plugin
//     method or function, including parameter types, defaults, and
//     documentation metadata.
//   - Method / Function: the closures that implement a plugin. Use the
//     typed wrappers (StringMethod, Int64Method, etc.) to avoid writing
//     receiver type-checks by hand.
//
// See the examples for a walkthrough of registering a plugin method.
package bloblangv2
