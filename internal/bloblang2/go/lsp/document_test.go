package lsp

import (
	"testing"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/pratt/syntax"
)

func TestDocumentStoreOpenAndGet(t *testing.T) {
	s := newDocumentStore()

	s.open("file:///a.blobl2", "output = input")

	text, prog, ok := s.get("file:///a.blobl2")
	if !ok {
		t.Fatal("expected document to exist after open")
	}
	if text != "output = input" {
		t.Errorf("text = %q, want %q", text, "output = input")
	}
	if prog != nil {
		t.Error("expected nil program before any parse")
	}
}

func TestDocumentStoreGetMissing(t *testing.T) {
	s := newDocumentStore()

	_, _, ok := s.get("file:///missing.blobl2")
	if ok {
		t.Error("expected missing document to return ok=false")
	}
}

func TestDocumentStoreUpdate(t *testing.T) {
	s := newDocumentStore()

	s.open("file:///a.blobl2", "old content")
	s.update("file:///a.blobl2", "new content")

	text, _, ok := s.get("file:///a.blobl2")
	if !ok {
		t.Fatal("expected document to exist after update")
	}
	if text != "new content" {
		t.Errorf("text = %q, want %q", text, "new content")
	}
}

func TestDocumentStoreUpdateCreatesIfMissing(t *testing.T) {
	s := newDocumentStore()

	s.update("file:///new.blobl2", "created via update")

	text, _, ok := s.get("file:///new.blobl2")
	if !ok {
		t.Fatal("expected document to exist after upsert")
	}
	if text != "created via update" {
		t.Errorf("text = %q, want %q", text, "created via update")
	}
}

func TestDocumentStoreClose(t *testing.T) {
	s := newDocumentStore()

	s.open("file:///a.blobl2", "content")
	s.close("file:///a.blobl2")

	_, _, ok := s.get("file:///a.blobl2")
	if ok {
		t.Error("expected document to be gone after close")
	}
}

func TestDocumentStoreCloseNonExistent(t *testing.T) {
	s := newDocumentStore()
	// Should not panic.
	s.close("file:///nope.blobl2")
}

func TestDocumentStoreSetProgram(t *testing.T) {
	s := newDocumentStore()
	s.open("file:///a.blobl2", "output = input")

	prog := &syntax.Program{}
	s.setProgram("file:///a.blobl2", prog)

	_, got, ok := s.get("file:///a.blobl2")
	if !ok {
		t.Fatal("expected document to exist")
	}
	if got != prog {
		t.Error("expected setProgram to store the program")
	}
}

func TestDocumentStoreSetProgramMissing(t *testing.T) {
	s := newDocumentStore()
	// Should not panic when setting program on non-existent document.
	s.setProgram("file:///missing.blobl2", &syntax.Program{})
}

func TestDocumentStoreMultipleDocuments(t *testing.T) {
	s := newDocumentStore()

	s.open("file:///a.blobl2", "aaa")
	s.open("file:///b.blobl2", "bbb")

	textA, _, okA := s.get("file:///a.blobl2")
	textB, _, okB := s.get("file:///b.blobl2")

	if !okA || textA != "aaa" {
		t.Errorf("doc a: ok=%v, text=%q", okA, textA)
	}
	if !okB || textB != "bbb" {
		t.Errorf("doc b: ok=%v, text=%q", okB, textB)
	}

	s.close("file:///a.blobl2")

	_, _, okA = s.get("file:///a.blobl2")
	_, _, okB = s.get("file:///b.blobl2")
	if okA {
		t.Error("doc a should be gone after close")
	}
	if !okB {
		t.Error("doc b should still exist")
	}
}
