package lsp

import (
	"testing"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/pratt/syntax"
)

func TestPosErrorToDiagnostic(t *testing.T) {
	tests := []struct {
		name     string
		err      syntax.PosError
		wantLine int
		wantCol  int
		wantMsg  string
	}{
		{
			name:     "normal 1-based to 0-based conversion",
			err:      syntax.PosError{Pos: syntax.Pos{Line: 5, Column: 10}, Msg: "undeclared variable $x"},
			wantLine: 4,
			wantCol:  9,
			wantMsg:  "undeclared variable $x",
		},
		{
			name:     "first position",
			err:      syntax.PosError{Pos: syntax.Pos{Line: 1, Column: 1}, Msg: "unexpected token"},
			wantLine: 0,
			wantCol:  0,
			wantMsg:  "unexpected token",
		},
		{
			name:     "zero pos clamped to zero",
			err:      syntax.PosError{Pos: syntax.Pos{Line: 0, Column: 0}, Msg: "bad"},
			wantLine: 0,
			wantCol:  0,
			wantMsg:  "bad",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := posErrorToDiagnostic(tt.err)
			if d.Range.Start.Line != tt.wantLine {
				t.Errorf("start line = %d, want %d", d.Range.Start.Line, tt.wantLine)
			}
			if d.Range.Start.Character != tt.wantCol {
				t.Errorf("start character = %d, want %d", d.Range.Start.Character, tt.wantCol)
			}
			if d.Range.End.Line != tt.wantLine {
				t.Errorf("end line = %d, want %d", d.Range.End.Line, tt.wantLine)
			}
			if d.Range.End.Character != tt.wantCol {
				t.Errorf("end character = %d, want %d", d.Range.End.Character, tt.wantCol)
			}
			if d.Severity != severityError {
				t.Errorf("severity = %d, want %d", d.Severity, severityError)
			}
			if d.Source != "bloblang2" {
				t.Errorf("source = %q, want %q", d.Source, "bloblang2")
			}
			if d.Message != tt.wantMsg {
				t.Errorf("message = %q, want %q", d.Message, tt.wantMsg)
			}
		})
	}
}

func TestDiagnose(t *testing.T) {
	tests := []struct {
		name       string
		source     string
		wantCount  int
		wantSubstr string // substring expected in first diagnostic message
	}{
		{
			name:      "valid mapping produces no diagnostics",
			source:    `output = input.name`,
			wantCount: 0,
		},
		{
			name:      "valid mapping with variable",
			source:    "$x = input.name\noutput = $x",
			wantCount: 0,
		},
		{
			name:       "parse error on invalid syntax",
			source:     `output = =`,
			wantCount:  1,
			wantSubstr: "expected expression",
		},
		{
			name:       "undeclared variable",
			source:     `output = $missing`,
			wantCount:  1,
			wantSubstr: "undeclared variable",
		},
		{
			name:       "unknown function",
			source:     `output = bogus()`,
			wantCount:  1,
			wantSubstr: "unknown function",
		},
		{
			name:       "arity mismatch",
			source:     `output = uuid_v4("extra")`,
			wantCount:  1,
			wantSubstr: "accepts at most",
		},
		{
			name:       "method arity mismatch - missing required arg",
			source:     `output = input.test.map()`,
			wantCount:  1,
			wantSubstr: "requires at least 1 arguments",
		},
		{
			name:       "method arity mismatch - too many args",
			source:     `output = input.test.encode("base64", "extra")`,
			wantCount:  1,
			wantSubstr: "accepts at most 1 arguments",
		},
		{
			name:      "method with optional args is fine",
			source:    `output = input.test.format_json()`,
			wantCount: 0,
		},
		{
			name:       "input inside map body",
			source:     "map foo(x) {\n  input\n}\noutput = foo(1)",
			wantCount:  1,
			wantSubstr: "cannot access input inside a map body",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer()
			uri := "file:///test.blobl2"
			s.docs.open(uri, tt.source)
			s.diagnose(uri)

			diags := s.lastDiagnostics(uri)
			if len(diags) != tt.wantCount {
				t.Errorf("got %d diagnostics, want %d", len(diags), tt.wantCount)
				for _, d := range diags {
					t.Logf("  diagnostic: %s", d.Message)
				}
				return
			}
			if tt.wantCount > 0 && tt.wantSubstr != "" {
				msg := diags[0].Message
				if !contains(msg, tt.wantSubstr) {
					t.Errorf("diagnostic message %q does not contain %q", msg, tt.wantSubstr)
				}
			}
		})
	}
}

func newTestServer() *testServer {
	s := NewServer(nullReader{}, &discardWriter{})
	return &testServer{Server: s, notifications: make(map[string][]diagnostic)}
}

type testServer struct {
	*Server
	notifications map[string][]diagnostic
}

// Override sendNotification to capture diagnostics without writing JSON-RPC.
func (ts *testServer) diagnose(uri string) {
	text, _, ok := ts.docs.get(uri)
	if !ok {
		return
	}

	var diagnostics []diagnostic

	prog, parseErrs := syntax.Parse(text, "", nil)
	if len(parseErrs) > 0 {
		for _, e := range parseErrs {
			diagnostics = append(diagnostics, posErrorToDiagnostic(e))
		}
		ts.notifications[uri] = diagnostics
		return
	}

	syntax.Optimize(prog)

	resolveErrs := syntax.Resolve(prog, syntax.ResolveOptions{
		Methods:   ts.stdlibMethods,
		Functions: ts.stdlibFunctions,
	})
	for _, e := range resolveErrs {
		diagnostics = append(diagnostics, posErrorToDiagnostic(e))
	}

	ts.docs.setProgram(uri, prog)

	if diagnostics == nil {
		diagnostics = []diagnostic{}
	}
	ts.notifications[uri] = diagnostics
}

func (ts *testServer) lastDiagnostics(uri string) []diagnostic {
	return ts.notifications[uri]
}

type nullReader struct{}

func (nullReader) Read([]byte) (int, error) { return 0, nil }

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
