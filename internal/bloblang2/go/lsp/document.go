package lsp

import (
	"sync"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/pratt/syntax"
)

// documentState holds the text and last successful parse of a document.
type documentState struct {
	text string
	prog *syntax.Program // nil if never parsed successfully
}

// documentStore is a thread-safe in-memory store of open documents.
type documentStore struct {
	mu   sync.Mutex
	docs map[string]*documentState
}

func newDocumentStore() *documentStore {
	return &documentStore{docs: make(map[string]*documentState)}
}

func (s *documentStore) open(uri, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.docs[uri] = &documentState{text: text}
}

func (s *documentStore) update(uri, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if doc, ok := s.docs[uri]; ok {
		doc.text = text
	} else {
		s.docs[uri] = &documentState{text: text}
	}
}

func (s *documentStore) close(uri string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.docs, uri)
}

func (s *documentStore) get(uri string) (string, *syntax.Program, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, ok := s.docs[uri]
	if !ok {
		return "", nil, false
	}
	return doc.text, doc.prog, true
}

func (s *documentStore) setProgram(uri string, prog *syntax.Program) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if doc, ok := s.docs[uri]; ok {
		doc.prog = prog
	}
}
