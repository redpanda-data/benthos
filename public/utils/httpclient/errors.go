// Copyright 2025 Redpanda Data, Inc.

package httpclient

import (
	"fmt"
	"strings"
)

// ErrUnexpectedHTTPRes is an error returned when an HTTP request returned an
// unexpected response.
type ErrUnexpectedHTTPRes struct {
	Code int
	S    string
	Body []byte
}

// Error returns the error message for [ErrUnexpectedHTTPRes].
func (e ErrUnexpectedHTTPRes) Error() string {
	body := strings.ReplaceAll(string(e.Body), "\n", "")
	return fmt.Sprintf("HTTP request returned unexpected response code (%d): %s, body: %s", e.Code, e.S, body)
}
