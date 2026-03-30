// Package agentexam provides a framework for running and scoring agent
// "exams" — isolated evaluations where an AI agent works inside a temporary
// clean room directory, and its output is scored by a user-supplied function.
package agentexam

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Exam defines a self-contained agent evaluation. It specifies what goes into
// a clean room directory, the prompt the agent receives, and how to score the
// agent's work after it finishes.
type Exam struct {
	// Name identifies this exam (used in result reporting and temp dir naming).
	Name string

	// Files to place in the clean room before the agent runs. Keys are
	// relative paths (e.g., "tests/math/add_000.input.json"). Values are
	// the file contents.
	Files map[string][]byte

	// Prompt is the text passed to the agent.
	Prompt string

	// Score is called after the agent finishes. It receives the parent
	// context, a Room for accessing clean room files, and the output
	// writer for logging (useful when nesting exams). Returns results.
	Score func(ctx context.Context, room *Room, output io.Writer) ([]Result, error)
}

// Room provides read access to the clean room directory after an agent has
// finished working in it.
type Room struct {
	dir string
}

// GetFile returns the contents of a file at the given relative path within
// the clean room. The second return value is false if the file does not exist.
func (r *Room) GetFile(relativePath string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(r.dir, relativePath))
	if err != nil {
		return "", false
	}
	return string(data), true
}

// GetFileJSON reads a file at the given relative path and JSON-decodes it
// into v. Returns an error if the file does not exist or contains invalid JSON.
func (r *Room) GetFileJSON(relativePath string, v any) error {
	data, err := os.ReadFile(filepath.Join(r.dir, relativePath))
	if err != nil {
		return fmt.Errorf("reading %s: %w", relativePath, err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("decoding %s: %w", relativePath, err)
	}
	return nil
}

// ListFiles returns all file paths in the clean room as relative paths,
// sorted lexicographically.
func (r *Room) ListFiles() []string {
	var files []string
	_ = filepath.WalkDir(r.dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		rel, relErr := filepath.Rel(r.dir, path)
		if relErr != nil {
			return nil
		}
		files = append(files, rel)
		return nil
	})
	sort.Strings(files)
	return files
}

// Dir returns the absolute path to the clean room directory. Prefer GetFile
// and ListFiles for typical use; Dir is available for cases that need direct
// filesystem access.
func (r *Room) Dir() string {
	return r.dir
}

// Result is the outcome of scoring a single item within an exam.
type Result struct {
	// ID uniquely identifies this item (e.g., "stdlib/strings_007").
	ID string

	// Group is an optional grouping key for aggregate statistics (e.g.,
	// category name). Empty string means ungrouped.
	Group string

	// Name is a human-readable label for this item.
	Name string

	// Score is a value between 0 and 1, where 1 means fully correct and 0
	// means completely wrong.
	Score float64

	// Error describes why the item lost points. Empty if Score is 1.
	Error string
}

// Options configures an exam run.
type Options struct {
	// Agent to use. Required.
	Agent Agent

	// Timeout per exam. Zero means no timeout.
	Timeout time.Duration

	// WorkDir overrides the temporary directory location. If empty, a
	// deterministic path under os.TempDir() is used, derived from the
	// exam name.
	WorkDir string

	// KeepDir prevents cleanup of the work directory after the run.
	KeepDir bool

	// Output receives agent output (conversation text, tool calls, etc.)
	// as it happens. If nil, agent output is discarded.
	Output io.Writer
}

// Run executes a single exam: creates the clean room, runs the agent, and
// scores the results.
func Run(ctx context.Context, exam *Exam, opts *Options) ([]Result, error) {
	if opts.Agent == nil {
		return nil, errors.New("agentexam: Options.Agent is required")
	}

	dir := opts.WorkDir
	if dir == "" {
		dir = stableWorkDir(exam.Name)
	}

	// Remove any stale data from a previous run, then create a fresh dir.
	_ = os.RemoveAll(dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("agentexam: creating work dir: %w", err)
	}
	if err := writeFiles(dir, exam.Files); err != nil {
		return nil, fmt.Errorf("agentexam: writing clean room: %w", err)
	}

	runCtx := ctx
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	output := opts.Output
	if output == nil {
		output = io.Discard
	}

	fmt.Fprintf(output, "=== exam: %s ===\n", exam.Name)
	fmt.Fprintf(output, "  agent:     %s\n", opts.Agent)
	fmt.Fprintf(output, "  files:     %d\n", len(exam.Files))
	fmt.Fprintf(output, "  work dir:  %s\n", dir)
	if opts.Timeout > 0 {
		fmt.Fprintf(output, "  timeout:   %s\n", opts.Timeout)
	}
	fmt.Fprintln(output)

	if err := opts.Agent.Run(runCtx, dir, exam.Prompt, output); err != nil {
		return nil, fmt.Errorf("agentexam: agent run: %w", err)
	}

	fmt.Fprintf(output, "\n=== scoring: %s ===\n\n", exam.Name)

	// Score.
	room := &Room{dir: dir}
	results, err := exam.Score(ctx, room, output)
	if err != nil {
		return nil, fmt.Errorf("agentexam: scoring: %w", err)
	}

	// Cleanup unless told to keep it.
	if !opts.KeepDir {
		_ = os.RemoveAll(dir)
	}

	return results, nil
}

// RunAll executes multiple exams sequentially, returning all results keyed by
// exam name.
func RunAll(ctx context.Context, exams []*Exam, opts *Options) (map[string][]Result, error) {
	out := make(map[string][]Result, len(exams))
	for _, exam := range exams {
		results, err := Run(ctx, exam, opts)
		if err != nil {
			return out, fmt.Errorf("exam %q: %w", exam.Name, err)
		}
		out[exam.Name] = results
	}
	return out, nil
}

// Summary holds aggregate statistics computed from a slice of Results.
type Summary struct {
	Total      int
	TotalScore float64
	Groups     map[string]GroupSummary
}

// GroupSummary holds aggregate statistics for a single group.
type GroupSummary struct {
	Total      int
	TotalScore float64
}

// Summarize computes aggregate statistics from a slice of results.
func Summarize(results []Result) *Summary {
	s := &Summary{Groups: map[string]GroupSummary{}}
	for _, r := range results {
		s.Total++
		s.TotalScore += r.Score
		if r.Group != "" {
			g := s.Groups[r.Group]
			g.Total++
			g.TotalScore += r.Score
			s.Groups[r.Group] = g
		}
	}
	return s
}

func stableWorkDir(name string) string {
	h := sha256.Sum256([]byte(name))
	dirName := fmt.Sprintf("agentexam-%x", h[:8])
	return filepath.Join(os.TempDir(), dirName)
}

func writeFiles(dir string, files map[string][]byte) error {
	for relPath, content := range files {
		absPath := filepath.Join(dir, relPath)
		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(absPath, content, 0o644); err != nil {
			return err
		}
	}
	return nil
}
