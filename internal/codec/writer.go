// Copyright 2025 Redpanda Data, Inc.

package codec

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/redpanda-data/benthos/v4/internal/docs"
)

// NewWriterDocs returns the field documentation for writer codecs.
func NewWriterDocs(name string) docs.FieldSpec {
	return docs.FieldString(
		name, "The way in which the bytes of messages should be written out into the output data stream. It's possible to write lines using a custom delimiter with the `delim:x` codec, where x is the character sequence custom delimiter.", "lines", "delim:\t", "delim:foobar",
	).HasAnnotatedOptions(
		"all-bytes", "Only applicable to file based outputs. Writes each message to a file in full, if the file already exists the old content is deleted.",
		"append", "Append each message to the output stream without any delimiter or special encoding.",
		"lines", "Append each message to the output stream followed by a line break.",
		"delim:x", "Append each message to the output stream followed by a custom delimiter.",
	).LinterBlobl("")
}

//------------------------------------------------------------------------------

// SuffixFn is a function which should be called by codec writers to determine
// when a custom suffix must be emitted by the writer codec.
type SuffixFn func(data []byte) ([]byte, bool)

// WriterConfig is a general configuration struct that covers all writer codecs.
type WriterConfig struct {
	Append bool
}

// GetWriter returns a codec writer.
func GetWriter(codec string) (sFn SuffixFn, appendMode bool, err error) {
	switch codec {
	case "all-bytes":
		return func(data []byte) ([]byte, bool) { return nil, false }, false, nil
	case "append":
		return customDelimSuffixFn(""), true, nil
	case "lines":
		return customDelimSuffixFn("\n"), true, nil
	}
	if strings.HasPrefix(codec, "delim:") {
		by := strings.TrimPrefix(codec, "delim:")
		if by == "" {
			return nil, false, errors.New("custom delimiter codec requires a non-empty delimiter")
		}
		return customDelimSuffixFn(by), true, nil
	}
	return nil, false, fmt.Errorf("codec was not recognised: %v", codec)
}

func customDelimSuffixFn(suffix string) SuffixFn {
	suffixB := []byte(suffix)
	return func(data []byte) ([]byte, bool) {
		if len(suffixB) == 0 {
			return nil, false
		}
		if !bytes.HasSuffix(data, suffixB) {
			return suffixB, true
		}
		return nil, false
	}
}
