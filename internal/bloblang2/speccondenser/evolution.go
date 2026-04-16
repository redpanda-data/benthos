package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/agentexam"
)

// scoredSpec holds a condensed spec together with its scoring results
// and aggregate score, used for ranking within a generation.
type scoredSpec struct {
	Spec        string
	PoolResults []poolResult
	Score       float64
}

// runEvolution executes the evolutionary loop and returns the winning
// condensed spec and its pool results. When population=1, survivors=1,
// generations=1, this is equivalent to the original single-condense flow.
func runEvolution(
	ctx context.Context,
	cfg *Config,
	specFiles map[string][]byte,
	tests []eligibleTest,
	pools []PoolConfig,
	output io.Writer,
) (string, []poolResult, error) {
	N := cfg.Condense.Population
	M := cfg.Condense.Survivors
	X := cfg.Condense.Generations

	// Generate initial population.
	fmt.Fprintf(os.Stderr, "generating %d initial condensed spec(s)\n", N)
	population, err := generateSpecs(ctx, cfg, specFiles, N, output)
	if err != nil {
		return "", nil, fmt.Errorf("generating initial specs: %w", err)
	}

	var best scoredSpec

	for gen := 1; gen <= X; gen++ {
		if X > 1 {
			fmt.Fprintf(os.Stderr, "\n=== generation %d/%d: scoring %d spec(s) ===\n", gen, X, len(population))
		}

		// Score all specs in the population.
		scored := make([]scoredSpec, len(population))
		for i, spec := range population {
			if N > 1 {
				fmt.Fprintf(os.Stderr, "scoring spec %d/%d (%d bytes)\n", i+1, len(population), len(spec))
			}
			poolResults, scoreErr := scoreCondensedSpec(ctx, spec, tests, pools, output)
			if scoreErr != nil {
				return "", nil, fmt.Errorf("generation %d, spec %d: %w", gen, i, scoreErr)
			}
			scored[i] = scoredSpec{
				Spec:        spec,
				PoolResults: poolResults,
				Score:       aggregateScore(poolResults),
			}
		}

		// Rank by score descending.
		sort.Slice(scored, func(i, j int) bool {
			return scored[i].Score > scored[j].Score
		})

		best = scored[0]

		if X > 1 || N > 1 {
			fmt.Fprintf(os.Stderr, "generation %d/%d results:\n", gen, X)
			for i, s := range scored {
				fmt.Fprintf(os.Stderr, "  #%d: %.1f%% (%d bytes)\n", i+1, s.Score*100, len(s.Spec))
			}
		}

		// Write artifacts for survivors of this generation.
		survivors := scored[:M]
		if X > 1 || N > 1 {
			genDir := filepath.Join(cfg.ArtifactDir, fmt.Sprintf("gen_%d", gen))
			for i, s := range survivors {
				specDir := filepath.Join(genDir, fmt.Sprintf("rank_%d", i+1))
				if err := writeArtifactTo(specDir, s.Spec, s.PoolResults); err != nil {
					fmt.Fprintf(os.Stderr, "warning: writing gen %d rank %d artifact: %v\n", gen, i+1, err)
				}
			}
		}

		// If last generation, we're done.
		if gen == X {
			break
		}

		// If population == survivors, no mutation is possible.
		if N == M {
			fmt.Fprintf(os.Stderr, "population == survivors; no improvement possible, stopping early\n")
			break
		}
		toGenerate := N - M

		fmt.Fprintf(os.Stderr, "keeping top %d, improving to %d new variant(s)\n", M, toGenerate)
		improved, improveErr := improveSpecs(ctx, cfg, specFiles, survivors, toGenerate, output)
		if improveErr != nil {
			return "", nil, fmt.Errorf("generation %d improvement: %w", gen, improveErr)
		}

		// New population = survivors + improved variants.
		population = make([]string, 0, N)
		for _, s := range survivors {
			population = append(population, s.Spec)
		}
		population = append(population, improved...)
	}

	return best.Spec, best.PoolResults, nil
}

// generateSpecs runs the condense agent n times in parallel, returning
// n condensed spec strings.
func generateSpecs(
	ctx context.Context,
	cfg *Config,
	specFiles map[string][]byte,
	n int,
	output io.Writer,
) ([]string, error) {
	specs := make([]string, n)
	eg, egCtx := errgroup.WithContext(ctx)
	sw := &syncWriter{w: output}

	for i := range n {
		eg.Go(func() error {
			var condensed string
			exam, err := buildCondenseExam(specFiles, &condensed)
			if err != nil {
				return fmt.Errorf("spec %d: %w", i, err)
			}
			exam.Name = fmt.Sprintf("condense-%d", i)

			agent, err := buildAgent(cfg.Condense.Agent)
			if err != nil {
				return fmt.Errorf("spec %d: building agent: %w", i, err)
			}

			results, err := agentexam.Run(egCtx, exam, &agentexam.Options{
				Agent:   agent,
				Timeout: cfg.Condense.Timeout,
				KeepDir: cfg.KeepDir,
				Output:  sw,
			})
			if err != nil {
				return fmt.Errorf("spec %d: %w", i, err)
			}
			if len(results) == 0 || results[0].Score < 1 {
				return fmt.Errorf("spec %d: condense exam failed — agent did not produce condensed_spec.md", i)
			}

			specs[i] = condensed
			fmt.Fprintf(os.Stderr, "generated spec %d/%d (%d bytes)\n", i+1, n, len(condensed))
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}
	return specs, nil
}

// aggregateScore computes the mean overall_score across all pools,
// matching the artifact's aggregate computation.
func aggregateScore(pools []poolResult) float64 {
	if len(pools) == 0 {
		return 0
	}
	var allRead, allWrite []agentexam.Result
	for _, pr := range pools {
		allRead = append(allRead, pr.ReadResults...)
		allWrite = append(allWrite, pr.WriteResults...)
	}
	a := buildPoolArtifact(poolResult{
		ReadResults:  allRead,
		WriteResults: allWrite,
	})
	return a.OverallScore
}

// improveSpecs generates count improved variants from the given survivors.
// Improvements are distributed across survivors round-robin and run in parallel.
func improveSpecs(
	ctx context.Context,
	cfg *Config,
	specFiles map[string][]byte,
	survivors []scoredSpec,
	count int,
	output io.Writer,
) ([]string, error) {
	if count == 0 {
		return nil, nil
	}

	improved := make([]string, count)
	eg, egCtx := errgroup.WithContext(ctx)
	sw := &syncWriter{w: output}

	for i := range count {
		parent := survivors[i%len(survivors)]

		eg.Go(func() error {
			var condensed string
			exam, err := buildImproveExam(specFiles, parent, &condensed)
			if err != nil {
				return fmt.Errorf("improve %d: %w", i, err)
			}
			exam.Name = fmt.Sprintf("improve-%d", i)

			agent, err := buildAgent(cfg.Condense.Agent)
			if err != nil {
				return fmt.Errorf("improve %d: building agent: %w", i, err)
			}

			results, err := agentexam.Run(egCtx, exam, &agentexam.Options{
				Agent:   agent,
				Timeout: cfg.Condense.Timeout,
				KeepDir: cfg.KeepDir,
				Output:  sw,
			})
			if err != nil {
				return fmt.Errorf("improve %d: %w", i, err)
			}
			if len(results) == 0 || results[0].Score < 1 {
				return fmt.Errorf("improve %d: agent did not produce condensed_spec.md", i)
			}

			improved[i] = condensed
			fmt.Fprintf(os.Stderr, "improved spec %d/%d (%d bytes)\n", i+1, count, len(condensed))
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}
	return improved, nil
}

// buildImproveExam builds an exam where the agent improves an existing
// condensed spec based on its scoring weaknesses.
func buildImproveExam(
	specFiles map[string][]byte,
	parent scoredSpec,
	condensedOut *string,
) (*agentexam.Exam, error) {
	files := make(map[string][]byte, len(specFiles)+1)
	for k, v := range specFiles {
		files[k] = v
	}
	files["condensed_spec.md"] = []byte(parent.Spec)

	prompt := buildImprovePrompt(parent)

	return &agentexam.Exam{
		Name:     "improve",
		UseFiles: true,
		Files:    files,
		Prompt:   prompt,
		Score: func(_ context.Context, room *agentexam.Room, _ io.Writer) ([]agentexam.Result, error) {
			spec, ok := room.GetFile("condensed_spec.md")
			if !ok {
				return nil, errors.New("agent did not produce condensed_spec.md")
			}
			*condensedOut = spec
			return []agentexam.Result{{
				ID:    "improve",
				Name:  "improved spec produced",
				Score: 1,
			}}, nil
		},
	}, nil
}

// buildImprovePrompt constructs the prompt for the improvement agent,
// including category-level scores to guide targeted improvements.
func buildImprovePrompt(parent scoredSpec) string {
	var allRead, allWrite []agentexam.Result
	for _, pr := range parent.PoolResults {
		allRead = append(allRead, pr.ReadResults...)
		allWrite = append(allWrite, pr.WriteResults...)
	}
	cats := buildCategoryScores(allRead, allWrite)

	type catEntry struct {
		name     string
		read     float64
		write    float64
		combined float64
	}
	var entries []catEntry
	for name, cs := range cats {
		combined := (cs.ReadScore + cs.WriteScore) / 2
		entries = append(entries, catEntry{name, cs.ReadScore, cs.WriteScore, combined})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].combined < entries[j].combined
	})

	var scoreReport strings.Builder
	for _, e := range entries {
		fmt.Fprintf(&scoreReport, "- %s: read=%.0f%% write=%.0f%% combined=%.0f%%\n",
			e.name, e.read*100, e.write*100, e.combined*100)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Task: Improve a Condensed Programming Language Specification\n\n")
	fmt.Fprintf(&b, "You have access to:\n")
	fmt.Fprintf(&b, "1. The complete Bloblang V2 specification in the spec/ directory (the authoritative source of truth)\n")
	fmt.Fprintf(&b, "2. A previously condensed version at condensed_spec.md (your starting point)\n\n")
	fmt.Fprintf(&b, "## Current Performance\n\n")
	fmt.Fprintf(&b, "The current condensed spec scored %.1f%% overall when tested. ", parent.Score*100)
	fmt.Fprintf(&b, "Here are the category-level scores (sorted from weakest to strongest):\n\n")
	b.WriteString(scoreReport.String())
	b.WriteString(improveInstructions)
	return b.String()
}

const improveInstructions = `
## Instructions

1. Use your available tools to list and read all files in the spec/ directory.
2. Read condensed_spec.md to understand the current condensed version.
3. Identify gaps, inaccuracies, or missing details in the condensed spec, focusing on the WEAKEST categories listed above.
4. Make targeted, minimal improvements to address the weakest areas. Do NOT rewrite the entire document.
5. Write the improved version to condensed_spec.md (overwriting the existing file).

## Improvement Rules

- Focus your changes on the categories with the LOWEST scores. These are the areas where another agent failed to correctly read or write mappings using only the condensed spec.
- A low READ score means the condensed spec is missing or misstating rules that are needed to mentally execute a mapping. Add or correct the relevant details.
- A low WRITE score means the condensed spec is missing or misstating rules that are needed to compose a mapping. Add or correct the relevant syntax, operators, or functions.
- Preserve everything that is already working well. Do NOT remove or substantially alter sections covering high-scoring categories.
- Keep the spec as compact as possible. Add detail only where the scores indicate it is needed.
- Do NOT add invented features or behaviors not in the original spec.
- Do NOT include test cases or examples unless they clarify an otherwise ambiguous rule.

## IMPORTANT

You MUST write the file condensed_spec.md before you finish. Do not stop until you have written the file.
`
