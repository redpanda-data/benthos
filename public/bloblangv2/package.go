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
//
// # Coexistence with V1
//
// V2 ships alongside V1 rather than replacing it. The two languages have
// separate plugin registries: methods and functions registered on a
// public/bloblangv2 Environment are not visible to V1 mappings, and
// vice versa. The V1 stdlib has been ported method-by-method to the V2
// surface under internal/impl; the per-method status, including any
// semantic shifts (e.g. variadic arguments folded into arrays, error
// object shape), is tracked in internal/bloblang2/PARITY.md.
//
// Host components select a language per field. A bloblang field uses
// the V1 environment and is linted via the V1 path; a bloblang_v2 field
// uses the V2 environment and is linted via LintBloblangV2Mapping in
// internal/docs. Components must pick one or the other — there is no
// "accept either" field type.
//
// One known gap: interpolated string fields (the ${! ... } form) still
// dispatch through the V1 environment only. Plugins registered as
// V2-only methods will not be available inside interpolated strings,
// even when the host component also exposes a bloblang_v2 mapping
// field. The remaining-work list in internal/bloblang2/REMAINING.md
// tracks this and other gaps.
//
// For migrating existing V1 mappings and configs to V2, see the
// public/bloblangv2/migrator (mapping-level) and public/service/migrator
// (config-level) packages.
package bloblangv2
