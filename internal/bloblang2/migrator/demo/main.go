package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"

	_ "embed"

	"gopkg.in/yaml.v3"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/pratt/eval"
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/pratt/syntax"
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/translator"
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/v1ast"
	"github.com/redpanda-data/benthos/v4/public/bloblang"
)

//go:embed page.html
var pageHTML []byte

// Shared demo assets live alongside the sibling demo; we serve them from
// disk so this demo stays in lockstep with the V2 build pipeline (ts and
// tree-sitter Taskfiles write into that directory).
var sharedDemoDir = func() string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "demo")
}()

// Cached at startup since they don't change.
var (
	stdlibMethods         map[string]syntax.MethodInfo
	stdlibFunctions       map[string]syntax.FunctionInfo
	stdlibMethodOpcodes   map[string]uint16
	stdlibFunctionOpcodes map[string]uint16
)

func init() {
	stdlibMethods, stdlibFunctions = eval.StdlibNames()
	stdlibMethodOpcodes, stdlibFunctionOpcodes = eval.StdlibOpcodes()
}

type executeRequest struct {
	V1Mapping string `json:"v1_mapping"`
	Input     string `json:"input"`
	Engine    string `json:"engine"` // "v1" or "v2"
}

type posError struct {
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Message string `json:"message"`
}

type translateNote struct {
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Severity string `json:"severity"`
	RuleID   string `json:"rule_id"`
	Message  string `json:"message"`
}

type executeResponse struct {
	V2Mapping      string          `json:"v2_mapping"`
	TranslateNotes []translateNote `json:"translate_notes,omitempty"`
	V1ParseErrors  []posError      `json:"v1_parse_errors,omitempty"`
	V2ParseErrors  []posError      `json:"v2_parse_errors,omitempty"`
	RuntimeError   string          `json:"runtime_error,omitempty"`
	Result         string          `json:"result,omitempty"`
}

func handleExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req executeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var resp executeResponse
	defer func() {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}()

	// 1. Translate V1 → V2 (always, so the UI's V2 pane updates).
	rep, cerr := migrateV1(req.V1Mapping)
	if rep != nil {
		resp.V2Mapping = rep.V2Mapping
		resp.TranslateNotes = notesFromReport(rep)
	}
	if cerr != nil {
		// V1 side failed to parse — surface that; there is no V2 to run.
		resp.V1ParseErrors = v1ParseErrorsOf(cerr)
		return
	}

	// 2. Parse input JSON (shared across engines).
	var inputVal any
	if err := json.Unmarshal([]byte(req.Input), &inputVal); err != nil {
		resp.RuntimeError = fmt.Sprintf("invalid input JSON: %v", err)
		return
	}

	// 3. Execute via the requested engine.
	switch req.Engine {
	case "", "v1":
		executeV1(req.V1Mapping, inputVal, &resp)
	case "v2":
		if resp.V2Mapping == "" {
			resp.RuntimeError = "no V2 mapping was produced (check translate notes)"
			return
		}
		executeV2(resp.V2Mapping, inputVal, &resp)
	default:
		resp.RuntimeError = fmt.Sprintf("unknown engine %q (expected v1 or v2)", req.Engine)
	}
}

// migrateV1 runs the translator with a default 0 coverage gate so the demo
// always gets a best-effort V2 mapping back, even for low-coverage inputs.
// A V1 parse failure is surfaced separately.
func migrateV1(v1Source string) (*translator.Report, error) {
	if strings.TrimSpace(v1Source) == "" {
		return &translator.Report{}, nil
	}
	opts := translator.Options{
		MinCoverage: 0, // never gate — we want to show whatever the translator can emit
		Verbose:     true,
	}
	rep, err := translator.Migrate(v1Source, opts)
	if err != nil {
		// CoverageError can still carry a partial report; other errors can't.
		var cerr *translator.CoverageError
		if errors.As(err, &cerr) && cerr.Report != nil {
			return cerr.Report, err
		}
		return nil, err
	}
	return rep, nil
}

func notesFromReport(rep *translator.Report) []translateNote {
	if rep == nil || len(rep.Changes) == 0 {
		return nil
	}
	out := make([]translateNote, len(rep.Changes))
	for i, c := range rep.Changes {
		out[i] = translateNote{
			Line:     c.Line,
			Column:   c.Column,
			Severity: c.Severity.String(),
			RuleID:   c.RuleID.String(),
			Message:  c.Explanation,
		}
	}
	return out
}

func v1ParseErrorsOf(err error) []posError {
	var pe *bloblang.ParseError
	if errors.As(err, &pe) {
		return []posError{{Line: pe.Line, Column: pe.Column, Message: pe.Error()}}
	}
	var v1pe *v1ast.ParseError
	if errors.As(err, &v1pe) {
		return []posError{{Line: v1pe.Pos.Line, Column: v1pe.Pos.Column, Message: v1pe.Msg}}
	}
	return []posError{{Line: 1, Column: 1, Message: err.Error()}}
}

func executeV1(mapping string, input any, resp *executeResponse) {
	if strings.TrimSpace(mapping) == "" {
		resp.Result = "null"
		return
	}
	exe, err := bloblang.Parse(mapping)
	if err != nil {
		resp.V1ParseErrors = v1ParseErrorsOf(err)
		return
	}
	out, err := exe.Query(input)
	if err != nil {
		if errors.Is(err, bloblang.ErrRootDeleted) {
			resp.Result = "< message deleted >"
			return
		}
		resp.RuntimeError = err.Error()
		return
	}
	resp.Result = jsonIndent(out, resp)
}

func executeV2(v2Source string, input any, resp *executeResponse) {
	prog, errs := syntax.Parse(v2Source, "", nil)
	if len(errs) > 0 {
		resp.V2ParseErrors = posErrorsFromSyntax(errs)
		return
	}
	syntax.Optimize(prog)
	if resolveErrs := syntax.Resolve(prog, syntax.ResolveOptions{
		Methods:         stdlibMethods,
		Functions:       stdlibFunctions,
		MethodOpcodes:   stdlibMethodOpcodes,
		FunctionOpcodes: stdlibFunctionOpcodes,
	}); len(resolveErrs) > 0 {
		resp.V2ParseErrors = posErrorsFromSyntax(resolveErrs)
		return
	}
	interp := eval.New(prog)
	interp.RegisterStdlib()
	interp.RegisterLambdaMethods()

	out, _, deleted, err := interp.Run(input, map[string]any{})
	if err != nil {
		resp.RuntimeError = err.Error()
		return
	}
	if deleted {
		resp.Result = "< message deleted >"
		return
	}
	resp.Result = jsonIndent(out, resp)
}

func jsonIndent(v any, resp *executeResponse) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		resp.RuntimeError = fmt.Sprintf("failed to marshal output: %v", err)
		return ""
	}
	return string(b)
}

type completionItem struct {
	Label string `json:"label"`
	Kind  string `json:"kind"`
}

var cachedCompletions []byte

func init() {
	var items []completionItem
	for name := range stdlibMethods {
		items = append(items, completionItem{Label: name, Kind: "method"})
	}
	for name := range stdlibFunctions {
		items = append(items, completionItem{Label: name, Kind: "function"})
	}
	cachedCompletions, _ = json.Marshal(items)
}

func handleCompletions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write(cachedCompletions)
}

// caseStudiesDir returns the absolute path to the V1 corpus case studies,
// derived from this file's location so `go run` works from any cwd.
func caseStudiesDir() string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(thisFile), "..", "v1spec", "tests", "case_studies")
}

type caseStudySpec struct {
	Description string          `yaml:"description"`
	Tests       []caseStudyTest `yaml:"tests"`
}

type caseStudyTest struct {
	Name    string `yaml:"name"`
	Mapping string `yaml:"mapping"`
	Input   any    `yaml:"input"`
}

type caseStudyItem struct {
	File        string `json:"file"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Mapping     string `json:"mapping"`
	Input       string `json:"input"`
}

func handleCaseStudies(w http.ResponseWriter, r *http.Request) {
	dir := caseStudiesDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		http.Error(w, "case studies not found", http.StatusNotFound)
		return
	}

	var items []caseStudyItem
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var spec caseStudySpec
		if err := yaml.Unmarshal(data, &spec); err != nil {
			continue
		}
		for _, t := range spec.Tests {
			if t.Mapping == "" {
				continue
			}
			inputJSON, err := json.MarshalIndent(t.Input, "", "  ")
			if err != nil {
				continue
			}
			items = append(items, caseStudyItem{
				File:        entry.Name(),
				Name:        t.Name,
				Description: strings.TrimSpace(spec.Description),
				Mapping:     t.Mapping,
				Input:       string(inputJSON),
			})
		}
	}

	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(items)
}

func posErrorsFromSyntax(errs []syntax.PosError) []posError {
	out := make([]posError, len(errs))
	for i, e := range errs {
		out[i] = posError{Line: e.Pos.Line, Column: e.Pos.Column, Message: e.Msg}
	}
	return out
}

// serveSharedAsset serves a file from the sibling demo directory. The Go
// build pipelines (ts/bundle.mjs, tree-sitter/Taskfile sync-demo) write
// these assets into that directory; this demo consumes them read-only.
func serveSharedAsset(name, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(sharedDemoDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			http.Error(w, fmt.Sprintf("%s not available (run the V2 build first): %v", name, err), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_, _ = w.Write(data)
	}
}

func openBrowser(url string) {
	var cmd string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "linux":
		cmd = "xdg-open"
	case "windows":
		cmd = "rundll32"
		_ = exec.Command(cmd, "url.dll,FileProtocolHandler", url).Start()
		return
	default:
		return
	}
	_ = exec.Command(cmd, url).Start()
}

func main() {
	addr := flag.String("addr", ":4196", "listen address")
	noOpen := flag.Bool("no-open", false, "don't open browser automatically")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(pageHTML)
	})
	mux.HandleFunc("/execute", handleExecute)
	mux.HandleFunc("/completions", handleCompletions)
	mux.HandleFunc("/case-studies", handleCaseStudies)
	mux.HandleFunc("/tree-sitter-bloblang2.wasm", serveSharedAsset("tree-sitter-bloblang2.wasm", "application/wasm"))
	mux.HandleFunc("/highlights.scm", serveSharedAsset("highlights.scm", "text/plain; charset=utf-8"))
	mux.HandleFunc("/bloblang2.mjs", serveSharedAsset("bloblang2.mjs", "application/javascript; charset=utf-8"))
	mux.HandleFunc("/bloblang2.mjs.map", serveSharedAsset("bloblang2.mjs.map", "application/json; charset=utf-8"))

	server := &http.Server{
		Addr:         *addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	if !*noOpen {
		openBrowser("http://localhost" + *addr)
	}

	log.Printf("Bloblang migrator demo server listening on http://localhost%s", *addr)
	log.Printf("WARNING: This server is for local demo purposes only. Do not expose to the internet.")

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan
		log.Println("Shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
}
