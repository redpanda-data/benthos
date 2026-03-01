package lsp

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
	"github.com/redpanda-data/benthos/v4/internal/cli/common"
	"github.com/redpanda-data/benthos/v4/internal/cli/lsp/linting"
	"github.com/redpanda-data/benthos/v4/internal/config"
	"github.com/redpanda-data/benthos/v4/internal/config/schema"
	"github.com/redpanda-data/benthos/v4/internal/docs"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

const (
	placeholder     = "__placeholder__"
	lintingDebounce = time.Second * 1
)

type stateManager struct {
	documents map[string]string
	opts      *common.CLIOpts
	schema    schema.Full
	linter    *linting.Debouncer
}

// newStateManager creates a new stateManger which is responsible for tracking opened connect.yaml files
// and inspecting them for various lsp tasks such as completion or documentation.
func newStateManager(opts *common.CLIOpts) stateManager {
	schema := schema.New(opts.Version, opts.DateBuilt, opts.Environment, opts.BloblEnvironment)
	schema.Config = opts.MainConfigSpecCtor()

	return stateManager{
		documents: map[string]string{},
		opts:      opts,
		schema:    schema,
		linter:    linting.NewDebouncer(opts),
	}
}

func (s *stateManager) didChange(context *glsp.Context, params *protocol.DidChangeTextDocumentParams) error {
	switch {
	case len(params.ContentChanges) > 0:
		if v, ok := params.ContentChanges[0].(protocol.TextDocumentContentChangeEventWhole); ok {
			s.documents[params.TextDocument.URI] = v.Text
			s.linter.Debounce(params.TextDocument.URI, lintingDebounce, func() {
				s.publishDiagnostics(context, params.TextDocument.URI, v.Text)
			})
		}
	}

	return nil
}

func countLeadingWhitespaceRunes(line string) int {
	trimmed := strings.TrimLeftFunc(line, unicode.IsSpace)
	return utf8.RuneCountInString(line) - utf8.RuneCountInString(trimmed)
}

func cleanToken(t string) string {
	x := strings.TrimSpace(t)
	return strings.TrimSuffix(x, ":")
}

type lineResult struct {
	config string
	value  string
}

func parseCurrentText(t string) *lineResult {
	vals := strings.Split(t, ":")
	if len(vals) > 1 {
		line := &lineResult{
			config: strings.TrimSpace(vals[0]),
			value:  strings.TrimSpace(vals[1]),
		}
		return line
	}

	return nil
}

func (s *stateManager) completion(context *glsp.Context, params *protocol.CompletionParams) (any, error) {
	doc, ok := s.documents[params.TextDocument.URI]
	if !ok {
		return nil, nil
	}

	var (
		parentToken string
		currentText string
	)

	// When doing completion we capture the doc in a half edited state, meaning it could be invalid.
	// To fix this, we replace the invalid line with a placeholder (ie: from "tab" to "tab: __placeholder__")
	lines := strings.Split(doc, "\n")
	if params.Position.Line >= 1 {
		parentToken = cleanToken(lines[params.Position.Line-1])
	}
	cursorLine := lines[params.Position.Line]
	currentText = cleanToken(cursorLine)

	// capture whitespace so we can ensure correct indentation after replacement of broken line
	prefix := strings.Repeat(" ", countLeadingWhitespaceRunes(cursorLine))
	lines[params.Position.Line] = fmt.Sprintf("%s%s: true", prefix, placeholder)

	// rejoin doc, it should now be valid for parsing
	validDoc := strings.Join(lines, "\n") + "\n"

	// Parse valid document and get AST
	astFile, err := parser.ParseBytes([]byte(validDoc), parser.ParseComments)
	if err != nil {
		return nil, err
	}

	res, err := s.parseSpecByFile(astFile, int(params.Position.Line+1), int(params.Position.Character))
	if err != nil {
		return nil, err
	}

	token := normaliseToken(parentToken)
	fmt.Printf("\n\n\n\nCleaned parent token: %s\n\n", token)
	fmt.Printf("\n\n\nCurrent text is: %s\n\n\n", currentText)

	var (
		components      []docs.ComponentSpec
		completionItems []protocol.CompletionItem
	)
	switch token {
	case "-":
		return completionItems, nil
	case "":
		format := protocol.InsertTextFormatSnippet
		for _, v := range []string{"input", "processors", "cache", "buffer", "http", "metrics", "output"} {
			insert := fmt.Sprintf("%s:\n\t", v)
			completionItems = append(completionItems, protocol.CompletionItem{
				Label:            v,
				Documentation:    "",
				InsertText:       &insert,
				InsertTextFormat: &format,
			})
		}
		return completionItems, nil
	case "input":
		components = s.schema.Inputs
	case "output":
		components = s.schema.Outputs
	case "processors":
		components = s.schema.Processors
	case "cache":
		components = s.schema.Caches
	case "buffer":
		components = s.schema.Caches
	case "metrics":
		components = s.schema.Metrics
		// case "http":
		// 	components = s.schema.
	}

	var additionalEdits []protocol.TextEdit
	switch {
	// components (http_server:, postgres_cdc: etc)
	case len(components) > 0:
		for _, component := range components {
			var sb strings.Builder
			format := protocol.InsertTextFormatSnippet
			fmt.Fprintf(&sb, "%s:\n", component.Name)
			cnt := 1
			for _, cfg := range component.Config.Children {
				if cfg.CheckRequired() {
					if cfg.Kind == docs.KindArray {
						fmt.Fprintf(&sb, "\t%s:\n\t\t- ${%d}\n", cfg.Name, cnt)
					} else {
						fmt.Fprintf(&sb, "\t%s: ${%d}\n", cfg.Name, cnt)
					}
					cnt++
				}
			}
			insertText := sb.String()
			completionItems = append(completionItems, protocol.CompletionItem{
				Label:               component.Name,
				Deprecated:          &component.Config.IsDeprecated,
				Documentation:       component.Summary,
				InsertText:          &insertText,
				AdditionalTextEdits: additionalEdits,
				InsertTextFormat:    &format,
				// Kind:          &kind,
			})
		}
	default:
		// field (ie: connection_string:, stream_snapshot:)
		line := parseCurrentText(currentText)
		for _, opt := range res.cs.Config.Children {
			// field value (ie: stream_snapshot: tr)
			if line != nil && line.config == opt.Name {
				kind := protocol.CompletionItemKindProperty
				switch opt.Type {
				case docs.FieldTypeString:
					// if opt.Kind == docs.KindArray {
					// 	insert := fmt.Sprintf("%s:\n\t", "boop")
					// 	return []protocol.CompletionItem{{Label: insert, Deprecated: &opt.IsDeprecated, Documentation: opt.Description, InsertText: &insert, Kind: &kind}}, nil
					// } else {
					val := ""
					return []protocol.CompletionItem{{Label: val, Deprecated: &opt.IsDeprecated, Documentation: opt.Description, InsertText: &val, Kind: &kind}}, nil
					// }
				case docs.FieldTypeBool:
					t := "true"
					f := "false"
					completions := []protocol.CompletionItem{
						{Label: t, Deprecated: &opt.IsDeprecated, Documentation: opt.Description, InsertText: &t, Kind: &kind},
						{Label: f, Deprecated: &opt.IsDeprecated, Documentation: opt.Description, InsertText: &f, Kind: &kind},
					}
					return completions, nil
				}

			}
			var (
				kind       protocol.CompletionItemKind
				insertText string
			)

			// fields (ie: include:, stream_snapshot: tr
			switch opt.Type {
			case docs.FieldTypeString:
				if opt.Kind == docs.KindArray {
					insertText = fmt.Sprintf("%s:\n      - ", opt.Name)
				} else {
					insertText = fmt.Sprintf("%s: ", opt.Name)
					if opt.Default != nil {
						d := *opt.Default
						if v, ok := d.(string); ok {
							insertText += v
						}
					}
				}
				kind = protocol.CompletionItemKindText
			case docs.FieldTypeObject:
				insertText = fmt.Sprintf("%s:\n      ", opt.Name)
				// cnt := 1
				// for _, child := range opt.Children {
				// 	insertText += fmt.Sprintf("      %s: ${%d}\n", child.Name, cnt)
				// 	cnt++
				// }
				kind = protocol.CompletionItemKindProperty
			default: // includes "object" and others
				insertText = fmt.Sprintf("%s: ", opt.Name)
				kind = protocol.CompletionItemKindProperty
			}

			completionItems = append(completionItems, protocol.CompletionItem{
				Label:         opt.Name,
				Deprecated:    &opt.IsDeprecated,
				Documentation: opt.Description,
				InsertText:    &insertText,
				Kind:          &kind,
			})
		}
	}

	return completionItems, nil
}

func (s *stateManager) openDocument(context *glsp.Context, params *protocol.DidOpenTextDocumentParams) error {
	s.documents[params.TextDocument.URI] = params.TextDocument.Text
	// Lint when document opens
	s.publishDiagnostics(context, params.TextDocument.URI, params.TextDocument.Text)
	return nil
}

func (s *stateManager) documentLink(context *glsp.Context, params *protocol.DocumentLink) (*protocol.DocumentLink, error) {
	uri := protocol.DocumentUri("https://docs.redpanda.com/redpanda-connect/components/inputs/otlp_http/")
	lnk := &protocol.DocumentLink{Target: &uri}
	return lnk, nil
}

func (s *stateManager) closeDocument(context *glsp.Context, params *protocol.DidCloseTextDocumentParams) error {
	doc := params.TextDocument.URI
	if _, ok := s.documents[doc]; ok {
		// fmt.Printf("Closing document %s", doc)
		delete(s.documents, params.TextDocument.URI)
		// Clear diagnostics
		context.Notify(protocol.ServerTextDocumentPublishDiagnostics, protocol.PublishDiagnosticsParams{
			URI:         doc,
			Diagnostics: []protocol.Diagnostic{},
		})
	}
	return nil
}

// publishDiagnostics lints the document and publishes diagnostics to the client
func (s *stateManager) publishDiagnostics(context *glsp.Context, uri string, doc string) {
	lints, err := config.LintYAMLBytes(s.linter.Cfg, []byte(doc))

	// fmt.Println(doc)
	var diagnostics []protocol.Diagnostic
	if err != nil {
		// fmt.Printf("There's an error: %s\n", err)
		// Handle parse errors
		severity := protocol.DiagnosticSeverityError
		diagnostics = append(diagnostics, protocol.Diagnostic{
			Range: protocol.Range{
				Start: protocol.Position{Line: 0, Character: 0},
				End:   protocol.Position{Line: 0, Character: 0},
			},
			Severity: &severity,
			Message:  fmt.Sprintf("Failed to parse config: %v", err),
		})
	} else {
		// fmt.Println("There's no error")
		// Convert lints to diagnostics
		for _, lint := range lints {
			severity := s.lintTypeToSeverity(lint.Type)
			diagnostics = append(diagnostics, protocol.Diagnostic{
				Range: protocol.Range{
					Start: protocol.Position{Line: uint32(lint.Line - 1), Character: uint32(lint.Column - 1)},
					End:   protocol.Position{Line: uint32(lint.Line - 1), Character: uint32(lint.Column)},
				},
				Severity: &severity,
				Message:  lint.What,
				Source:   stringPtr("connect"),
			})
		}
	}

	// Publish diagnostics to the client
	p := protocol.PublishDiagnosticsParams{URI: uri, Diagnostics: make([]protocol.Diagnostic, 0)}
	if len(diagnostics) > 0 {
		p.Diagnostics = diagnostics
	}
	context.Notify(protocol.ServerTextDocumentPublishDiagnostics, p)
}

// lintTypeToSeverity converts a lint type to an LSP diagnostic severity
func (s *stateManager) lintTypeToSeverity(lintType docs.LintType) protocol.DiagnosticSeverity {
	switch lintType {
	case docs.LintFailedRead,
		docs.LintComponentMissing,
		docs.LintComponentNotFound,
		docs.LintMissing,
		docs.LintExpectedArray,
		docs.LintExpectedObject,
		docs.LintExpectedScalar,
		docs.LintBadBloblang:
		return protocol.DiagnosticSeverityError
	case docs.LintDeprecated,
		docs.LintShouldOmit:
		return protocol.DiagnosticSeverityWarning
	case docs.LintMissingEnvVar,
		docs.LintUnknown,
		docs.LintInvalidOption:
		return protocol.DiagnosticSeverityWarning
	default:
		return protocol.DiagnosticSeverityInformation
	}
}

// stringPtr returns a pointer to a string
func stringPtr(s string) *string {
	return &s
}

// func (s *state) onHover(id int, uri string, position protocol.Position) {
func (s *stateManager) onHover(context *glsp.Context, params *protocol.HoverParams) (*protocol.Hover, error) {
	doc, ok := s.documents[params.TextDocument.URI]
	if !ok {
		return nil, nil
	}

	var (
		file *ast.File
		err  error
	)
	if file, err = parser.ParseBytes([]byte(doc), parser.ParseComments); err != nil {
		return nil, err
	}

	// $.input.generate.interval
	var token *TokenWithPath
	if token, err = findTokenAtPosition(file, int(params.Position.Line+1), int(params.Position.Character+1)); err != nil {
		return nil, err
	}

	path := strings.Split(token.Path, ".")
	// fmt.Printf("Token path: %s\n", token.Path)

	if path[0] != "$" {
		return &protocol.Hover{Contents: ""}, nil
	}

	var (
		components []docs.ComponentSpec
	)

	// $.input.generate.interval
	// $.input.generate.mapping
	var (
		cs docs.ComponentSpec
		fs docs.FieldSpec
	)
	path = path[1:]
	cnt := 0
	for _, node := range path {
		cnt++

		token := normaliseToken(node)
		switch token {
		case "input":
			components = s.schema.Inputs
		case "output":
			components = s.schema.Outputs
		case "processors":
			components = s.schema.Processors
		case "cache":
			components = s.schema.Caches
		case "buffer":
			components = s.schema.Caches
		case "metrics":
			components = s.schema.Metrics
		}

		// components
		for _, spec := range components {
			if node == spec.Name {
				cs = spec
				break
			}
		}

		// children
		if len(cs.Config.Children) > 0 {
			for _, c := range cs.Config.Children {
				if node == c.Name {
					fs = c
					break
				}
			}
		}

		if cnt == len(path) {
			switch node {
			// configs (url, etc)
			case fs.Name:
				content := fmt.Sprintf("# Field: `%s (%s)`\n-----------------------------\n%s", fs.Name, fs.Type, fs.Description)
				if len(fs.Examples) > 0 {
					content += fmt.Sprintf("\n-----------------------------\n# Example: `%s`", fs.Examples[0])
				}
				content += fmt.Sprintf("\n-----------------------------\n[%s field on docs.redpanda.com](https://docs.redpanda.com/redpanda-connect/components/%ss/%s#%s)\n", fs.Name, cs.Type, cs.Name, fs.Name)
				return &protocol.Hover{Contents: content}, nil
				// components (http_server, etc)
			case cs.Name:
				content := fmt.Sprintf("# Component: `%s (%s)`\n-----------------------------\n%s\n", cs.Name, cs.Type, cs.Summary)
				content += fmt.Sprintf("\n-----------------------------\n[`%s` on docs.redpanda.com](https://docs.redpanda.com/redpanda-connect/components/%ss/%s)\n", cs.Name, cs.Type, cs.Name)
				return &protocol.Hover{Contents: content}, nil
			}
		}
	}

	return &protocol.Hover{Contents: ""}, nil
}

type specResult struct {
	cs *docs.ComponentSpec
	fs *docs.FieldSpec
}

func (s *stateManager) parseSpecByFile(file *ast.File, line, character int) (specResult, error) {
	var (
		token *TokenWithPath
		err   error
	)
	// $.input.generate.mapping
	if token, err = findTokenAtPosition(file, line, character); err != nil {
		return specResult{}, err
	}

	path := strings.Split(token.Path, ".")
	// fmt.Printf("Token path: %s\n", token.Path)

	if path[0] != "$" {
		return specResult{}, errors.New("could not find token at position")
	}

	var (
		components []docs.ComponentSpec
		cs         docs.ComponentSpec
		fs         docs.FieldSpec
	)

	path = path[1:]
	cnt := 0
	for _, node := range path {
		cnt++

		token := normaliseToken(node)
		// fmt.Printf("Cleaned token: %s\n", token)
		switch token {
		case "input":
			components = s.schema.Inputs
		case "output":
			components = s.schema.Outputs
		case "processors":
			components = s.schema.Processors
		case "cache":
			components = s.schema.Caches
		case "buffer":
			components = s.schema.Caches
		case "metrics":
			components = s.schema.Metrics
		}

		// components
		for _, spec := range components {
			if node == spec.Name {
				cs = spec
				break
			}
		}

		// children
		if len(cs.Config.Children) > 0 {
			for _, c := range cs.Config.Children {
				if node == c.Name {
					fs = c
					break
				}
			}
		}
	}

	return specResult{&cs, &fs}, nil
}

// normaliseToken cleans processors[0] or processors[1] and returns "processors"
func normaliseToken(token string) string {
	const procToken = "processors"
	if strings.HasPrefix(token, procToken) {
		return procToken
	}
	return token
}
