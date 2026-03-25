package lsp

import (
	"fmt"
	"sort"
	"strings"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/pratt/syntax"
)

// completionEngine provides completion items for Bloblang V2.
type completionEngine struct {
	keywords  []completionItem
	functions []completionItem
	methods   []completionItem
}

func newCompletionEngine(methods map[string]bool, functions map[string]syntax.FunctionInfo) *completionEngine {
	e := &completionEngine{}

	// Keywords.
	for _, kw := range []string{
		"if", "else", "match", "as", "map", "import",
		"input", "output", "true", "false", "null",
	} {
		kind := completionKindKeyword
		if kw == "true" || kw == "false" || kw == "null" {
			kind = completionKindValue
		}
		e.keywords = append(e.keywords, completionItem{
			Label: kw,
			Kind:  kind,
		})
	}

	// Global functions.
	names := make([]string, 0, len(functions))
	for name := range functions {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fi := functions[name]
		detail := formatFunctionArity(name, fi)
		e.functions = append(e.functions, completionItem{
			Label:      name,
			Kind:       completionKindFunction,
			Detail:     detail,
			InsertText: name + "($0)",
		})
	}

	// Methods.
	methodNames := make([]string, 0, len(methods))
	for name := range methods {
		methodNames = append(methodNames, name)
	}
	sort.Strings(methodNames)
	for _, name := range methodNames {
		e.methods = append(e.methods, completionItem{
			Label:      name,
			Kind:       completionKindMethod,
			InsertText: name + "($0)",
		})
	}

	return e
}

func formatFunctionArity(name string, fi syntax.FunctionInfo) string {
	if fi.Total == 0 {
		return name + "()"
	}
	if fi.Required == fi.Total {
		return fmt.Sprintf("%s(%d args)", name, fi.Required)
	}
	return fmt.Sprintf("%s(%d to %d args)", name, fi.Required, fi.Total)
}

func (e *completionEngine) complete(text string, prog *syntax.Program, pos position, ctx *completionContext) []completionItem {
	trigger := ""
	if ctx != nil {
		trigger = ctx.TriggerCharacter
	}

	// Determine trigger from the character before the cursor if not provided.
	if trigger == "" && pos.Character > 0 {
		lines := strings.Split(text, "\n")
		if pos.Line < len(lines) {
			line := lines[pos.Line]
			if pos.Character <= len(line) {
				ch := line[pos.Character-1]
				switch ch {
				case '.':
					trigger = "."
				case '$':
					trigger = "$"
				case '@':
					trigger = "@"
				}
			}
		}
	}

	switch trigger {
	case ".":
		return e.methods
	case "$":
		return e.variableCompletions(prog)
	case "@":
		// After @ (metadata access) — no specific completions.
		return nil
	default:
		return e.generalCompletions(prog)
	}
}

// variableCompletions returns variables from the last successful parse.
func (e *completionEngine) variableCompletions(prog *syntax.Program) []completionItem {
	if prog == nil {
		return nil
	}

	seen := make(map[string]bool)

	for _, stmt := range prog.Stmts {
		collectVariables(stmt, seen)
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)

	var items []completionItem
	for _, name := range names {
		items = append(items, completionItem{
			Label: "$" + name,
			Kind:  completionKindVariable,
		})
	}
	return items
}

// collectVariables walks statements to find variable assignments.
func collectVariables(stmt syntax.Stmt, seen map[string]bool) {
	switch s := stmt.(type) {
	case *syntax.Assignment:
		if s.Target.Root == syntax.AssignVar && s.Target.VarName != "" {
			seen[s.Target.VarName] = true
		}
	case *syntax.IfStmt:
		for _, b := range s.Branches {
			for _, inner := range b.Body {
				collectVariables(inner, seen)
			}
		}
		for _, inner := range s.Else {
			collectVariables(inner, seen)
		}
	case *syntax.MatchStmt:
		for _, c := range s.Cases {
			if body, ok := c.Body.([]syntax.Stmt); ok {
				for _, inner := range body {
					collectVariables(inner, seen)
				}
			}
		}
	}
}

// generalCompletions returns keywords, functions, and map names.
func (e *completionEngine) generalCompletions(prog *syntax.Program) []completionItem {
	items := make([]completionItem, 0, len(e.keywords)+len(e.functions)+10)
	items = append(items, e.keywords...)
	items = append(items, e.functions...)

	// Add user-defined map names.
	if prog != nil {
		for _, m := range prog.Maps {
			items = append(items, completionItem{
				Label:      m.Name,
				Kind:       completionKindFunction,
				Detail:     "map " + m.Name,
				InsertText: m.Name + "($0)",
			})
		}
	}

	return items
}
