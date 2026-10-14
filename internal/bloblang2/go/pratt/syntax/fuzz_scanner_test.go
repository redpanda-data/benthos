package syntax

import "testing"

// FuzzScanner drives the tokenizer with arbitrary bytes and verifies it
// always terminates without panicking and within a bounded token count.
//
// Each call to scanner.next() must consume at least one byte of source or
// emit EOF. So total tokens cannot exceed input length by more than a small
// constant; we allow generous headroom and treat anything beyond as a
// likely infinite loop.
func FuzzScanner(f *testing.F) {
	for _, s := range loadFuzzCorpus(f) {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		if len(src) > fuzzMaxInputSize {
			return
		}
		s := newScanner(src, "")
		maxTokens := len(src)*8 + 64
		for range maxTokens {
			if s.next().Type == EOF {
				return
			}
		}
		t.Fatalf("scanner produced > %d tokens for %d-byte input — possible infinite loop", maxTokens, len(src))
	})
}
