// Copyright 2026 Redpanda Data, Inc.

package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestScannerSourceDetailsNilGetters asserts that the getters are safe to call
// on a nil receiver, returning zero values. Details are optional throughout
// the scanner APIs, so a nil pointer must behave like empty details.
func TestScannerSourceDetailsNilGetters(t *testing.T) {
	var details *ScannerSourceDetails

	assert.Empty(t, details.Name())
	assert.Zero(t, details.SizeHint())
}
