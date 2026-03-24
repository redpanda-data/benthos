package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	_ "embed"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/pratt/eval"
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/pratt/syntax"
)

//go:embed page.html
var pageHTML []byte

//go:embed tree-sitter-bloblang2.wasm
var treeSitterWASM []byte

//go:embed highlights.scm
var highlightsSCM []byte

// Cached at startup since they don't change.
var (
	stdlibMethods   map[string]bool
	stdlibFunctions map[string]syntax.FunctionInfo
)

func init() {
	stdlibMethods, stdlibFunctions = eval.StdlibNames()
}

type executeRequest struct {
	Mapping string `json:"mapping"`
	Input   string `json:"input"`
}

type posError struct {
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Message string `json:"message"`
}

type executeResponse struct {
	Result       string     `json:"result,omitempty"`
	ParseErrors  []posError `json:"parse_errors,omitempty"`
	RuntimeError string     `json:"runtime_error,omitempty"`
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

	// 1. Parse.
	prog, errs := syntax.Parse(req.Mapping, "", nil)
	if len(errs) > 0 {
		resp.ParseErrors = posErrorsFromSyntax(errs)
		return
	}

	// 2. Optimize.
	syntax.Optimize(prog)

	// 3. Resolve.
	if resolveErrs := syntax.Resolve(prog, stdlibMethods, stdlibFunctions); len(resolveErrs) > 0 {
		resp.ParseErrors = posErrorsFromSyntax(resolveErrs)
		return
	}

	// 4. Parse input JSON.
	var inputVal any
	if err := json.Unmarshal([]byte(req.Input), &inputVal); err != nil {
		resp.RuntimeError = fmt.Sprintf("invalid input JSON: %v", err)
		return
	}

	// 5. Execute.
	interp := eval.New(prog)
	interp.RegisterStdlib()
	interp.RegisterLambdaMethods()

	output, _, deleted, err := interp.Run(inputVal, map[string]any{})
	if err != nil {
		resp.RuntimeError = err.Error()
		return
	}
	if deleted {
		resp.Result = "< message deleted >"
		return
	}

	outBytes, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		resp.RuntimeError = fmt.Sprintf("failed to marshal output: %v", err)
		return
	}
	resp.Result = string(outBytes)
}

type completionItem struct {
	Label string `json:"label"`
	Kind  string `json:"kind"` // "method" or "function"
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

func posErrorsFromSyntax(errs []syntax.PosError) []posError {
	out := make([]posError, len(errs))
	for i, e := range errs {
		out[i] = posError{
			Line:    e.Pos.Line,
			Column:  e.Pos.Column,
			Message: e.Msg,
		}
	}
	return out
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
	addr := flag.String("addr", ":4195", "listen address")
	noOpen := flag.Bool("no-open", false, "don't open browser automatically")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(pageHTML)
	})
	mux.HandleFunc("/execute", handleExecute)
	mux.HandleFunc("/completions", handleCompletions)
	mux.HandleFunc("/tree-sitter-bloblang2.wasm", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/wasm")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_, _ = w.Write(treeSitterWASM)
	})
	mux.HandleFunc("/highlights.scm", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_, _ = w.Write(highlightsSCM)
	})

	server := &http.Server{
		Addr:         *addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	if !*noOpen {
		openBrowser("http://localhost" + *addr)
	}

	log.Printf("Bloblang V2 demo server listening on http://localhost%s", *addr)
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
