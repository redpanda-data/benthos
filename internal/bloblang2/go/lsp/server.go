package lsp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/pratt/eval"
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/pratt/syntax"
)

// Server is a minimal LSP server for Bloblang V2.
type Server struct {
	in  *bufio.Reader
	out io.Writer
	mu  sync.Mutex // protects writes to out

	docs       *documentStore
	completion *completionEngine

	// Cached stdlib metadata.
	stdlibMethods   map[string]bool
	stdlibFunctions map[string]syntax.FunctionInfo

	// Debounce timers per URI.
	timersMu sync.Mutex
	timers   map[string]*time.Timer

	shutdown bool
	logger   *log.Logger
}

// NewServer creates a new LSP server reading from in and writing to out.
func NewServer(in io.Reader, out io.Writer) *Server {
	methods, functions := eval.StdlibNames()
	s := &Server{
		in:              bufio.NewReader(in),
		out:             out,
		docs:            newDocumentStore(),
		stdlibMethods:   methods,
		stdlibFunctions: functions,
		timers:          make(map[string]*time.Timer),
		logger:          log.New(os.Stderr, "[bloblang2-lsp] ", log.LstdFlags),
	}
	s.completion = newCompletionEngine(methods, functions)
	return s
}

// Run starts the server loop. It blocks until the connection is closed.
func (s *Server) Run() error {
	for {
		msg, err := s.readMessage()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("read: %w", err)
		}
		s.handleMessage(msg)
		if msg.Method == "exit" {
			if s.shutdown {
				return nil
			}
			return errors.New("exit without shutdown")
		}
	}
}

func (s *Server) readMessage() (*jsonrpcMessage, error) {
	// Read headers.
	var contentLength int
	for {
		line, err := s.in.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break // end of headers
		}
		if strings.HasPrefix(line, "Content-Length:") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
			contentLength, err = strconv.Atoi(val)
			if err != nil {
				return nil, fmt.Errorf("invalid Content-Length: %w", err)
			}
		}
	}
	if contentLength == 0 {
		return nil, errors.New("missing Content-Length header")
	}

	// Read body.
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(s.in, body); err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var msg jsonrpcMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	return &msg, nil
}

func (s *Server) sendResponse(id json.RawMessage, result any) {
	s.writeMessage(&jsonrpcMessage{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	})
}

func (s *Server) sendNotification(method string, params any) {
	raw, _ := json.Marshal(params)
	s.writeMessage(&jsonrpcMessage{
		JSONRPC: "2.0",
		Method:  method,
		Params:  raw,
	})
}

func (s *Server) writeMessage(msg *jsonrpcMessage) {
	body, err := json.Marshal(msg)
	if err != nil {
		s.logger.Printf("marshal error: %v", err)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	if _, err := io.WriteString(s.out, header); err != nil {
		s.logger.Printf("write header error: %v", err)
		return
	}
	if _, err := s.out.Write(body); err != nil {
		s.logger.Printf("write body error: %v", err)
	}
}

func (s *Server) handleMessage(msg *jsonrpcMessage) {
	switch msg.Method {
	case "initialize":
		s.handleInitialize(msg)
	case "initialized":
		// no-op
	case "shutdown":
		s.shutdown = true
		s.sendResponse(msg.ID, nil)
	case "exit":
		// Handled by Run() loop — exit is signalled via s.shutdown.
	case "textDocument/didOpen":
		s.handleDidOpen(msg)
	case "textDocument/didChange":
		s.handleDidChange(msg)
	case "textDocument/didClose":
		s.handleDidClose(msg)
	case "textDocument/completion":
		s.handleCompletion(msg)
	default:
		// Unknown method — if it has an ID it's a request, respond with method not found.
		if msg.ID != nil {
			s.writeMessage(&jsonrpcMessage{
				JSONRPC: "2.0",
				ID:      msg.ID,
				Error:   &jsonrpcError{Code: -32601, Message: "method not found: " + msg.Method},
			})
		}
	}
}

func (s *Server) handleInitialize(msg *jsonrpcMessage) {
	s.sendResponse(msg.ID, initializeResult{
		Capabilities: serverCapabilities{
			TextDocumentSync: textDocumentSyncOptions{
				OpenClose: true,
				Change:    1, // Full
			},
			CompletionProvider: &completionOptions{
				TriggerCharacters: []string{".", "$", "@"},
			},
		},
		ServerInfo: serverInfo{
			Name:    "bloblang2-lsp",
			Version: "0.1.0",
		},
	})
}

func (s *Server) handleDidOpen(msg *jsonrpcMessage) {
	var params didOpenTextDocumentParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.logger.Printf("didOpen unmarshal: %v", err)
		return
	}
	s.docs.open(params.TextDocument.URI, params.TextDocument.Text)
	s.diagnose(params.TextDocument.URI)
}

func (s *Server) handleDidChange(msg *jsonrpcMessage) {
	var params didChangeTextDocumentParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.logger.Printf("didChange unmarshal: %v", err)
		return
	}
	if len(params.ContentChanges) > 0 {
		// Full sync: take the last change event.
		text := params.ContentChanges[len(params.ContentChanges)-1].Text
		s.docs.update(params.TextDocument.URI, text)
		s.debounceDiagnose(params.TextDocument.URI)
	}
}

func (s *Server) handleDidClose(msg *jsonrpcMessage) {
	var params didCloseTextDocumentParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.logger.Printf("didClose unmarshal: %v", err)
		return
	}
	// Clear diagnostics before closing.
	s.sendNotification("textDocument/publishDiagnostics", publishDiagnosticsParams{
		URI:         params.TextDocument.URI,
		Diagnostics: []diagnostic{},
	})
	s.docs.close(params.TextDocument.URI)
}

func (s *Server) handleCompletion(msg *jsonrpcMessage) {
	var params completionParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.logger.Printf("completion unmarshal: %v", err)
		s.sendResponse(msg.ID, []completionItem{})
		return
	}

	text, prog, ok := s.docs.get(params.TextDocument.URI)
	if !ok {
		s.sendResponse(msg.ID, []completionItem{})
		return
	}

	items := s.completion.complete(text, prog, params.Position, params.Context)
	s.sendResponse(msg.ID, items)
}

// debounceDiagnose schedules a diagnosis for the given URI after a short delay.
func (s *Server) debounceDiagnose(uri string) {
	s.timersMu.Lock()
	defer s.timersMu.Unlock()

	if t, ok := s.timers[uri]; ok {
		t.Stop()
	}
	s.timers[uri] = time.AfterFunc(80*time.Millisecond, func() {
		s.diagnose(uri)
	})
}
