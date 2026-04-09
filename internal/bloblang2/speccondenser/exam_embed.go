package main

import "embed"

// examFS contains the curated exam test suite used as the default when
// tests_dir is not configured. Files are under exam/*.yaml.
//
//go:embed exam/*.yaml
var examFS embed.FS
