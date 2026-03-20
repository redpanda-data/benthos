// Package llmtest provides an LLM-based testing framework that uses language
// models as judges to evaluate test outputs holistically. This is useful when
// precise assertions are insufficient and a more nuanced, human-like judgement
// is needed.
//
// The user prompt is where all judge tuning belongs (leniency, focus areas,
// style preferences, etc.) — the system prompt is not customizable.
//
// Models used with tool opts (e.g. OptToolOnFolder) must support tool calling.
//
// TODO: Note deduplication — when multiple judges produce overlapping notes,
// use an additional LLM call to deduplicate and summarize them.
//
// TODO: Additional backends — Claude (Anthropic API), OpenAI, etc. The
// Provider interface is the extension point for this.
package llmtest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
)

const systemPrompt = `You are a strict, objective test judge. Your role is to evaluate test input against judgement criteria provided by the user.

Instructions:
1. Read and understand the user's test input and judgement criteria carefully.
2. If file-reading tools are available, use them to inspect all relevant files before making your judgement.
3. Evaluate how closely the test input matches the judgement criteria.
4. Produce your response as a JSON object with exactly two fields:
   - "score": an integer from 0 to 100, where 100 means perfect alignment with the criteria and 0 means complete failure.
   - "notes": an array of strings, each a concise note explaining your reasoning.
5. Be strict and objective. Do not give the benefit of the doubt. Only award high scores when the criteria are clearly and fully met.`

// EvalOptions configures a judgement evaluation.
type EvalOptions struct {
	// Prompt is the user-supplied test input and judgement criteria.
	Prompt string
	// Tools are optional tool specs to expose to the judge (e.g. from
	// OptToolOnFolder). Models must support tool calling to use these.
	Tools []Tool
	// Judges is the number of independent judge invocations. Defaults to 1.
	Judges int
	// MinJudges is the minimum number of successful judges required for the
	// evaluation to succeed. Defaults to Judges (fail-fast). Set lower to
	// tolerate transient failures.
	MinJudges int
	// DebugWriter, if set, receives debug output of the full LLM exchange
	// from each judge. Nil means no debug output.
	DebugWriter io.Writer
}

// Result is the aggregated outcome of an evaluation with one or more judges.
type Result struct {
	// Scores from each successful judge, in order.
	Scores []int
	// Notes from each successful judge, indexed to match Scores.
	Notes [][]string
	// Errors from judges that failed (may be nil if all succeeded).
	Errors []error
	// Stats contains aggregate statistics over successful judges.
	Stats Stats
}

// Eval runs the judgement. It sends the prompt to Judges independent instances
// of the provider concurrently, collects results, and returns the aggregate.
// The evaluation succeeds as long as at least MinJudges return successfully.
func Eval(ctx context.Context, provider Provider, opts EvalOptions) (*Result, error) {
	if opts.Prompt == "" {
		return nil, errors.New("llmtest: prompt is required")
	}
	if opts.Judges <= 0 {
		opts.Judges = 1
	}
	if opts.MinJudges <= 0 {
		opts.MinJudges = opts.Judges
	}
	if opts.MinJudges > opts.Judges {
		return nil, fmt.Errorf("llmtest: MinJudges (%d) cannot exceed Judges (%d)", opts.MinJudges, opts.Judges)
	}

	req := JudgeRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   opts.Prompt,
		Tools:        opts.Tools,
		DebugWriter:  opts.DebugWriter,
	}

	type judgeResult struct {
		index int
		resp  *JudgeResponse
		err   error
	}

	results := make([]judgeResult, opts.Judges)

	var wg sync.WaitGroup
	wg.Add(opts.Judges)
	for i := range opts.Judges {
		go func(idx int) {
			defer wg.Done()
			resp, err := provider.Judge(ctx, req)
			results[idx] = judgeResult{index: idx, resp: resp, err: err}
		}(i)
	}
	wg.Wait()

	var (
		scores []int
		notes  [][]string
		errs   []error
	)
	for _, r := range results {
		if r.err != nil {
			errs = append(errs, fmt.Errorf("judge %d: %w", r.index, r.err))
		} else {
			scores = append(scores, r.resp.Score)
			notes = append(notes, r.resp.Notes)
		}
	}

	if len(scores) < opts.MinJudges {
		return nil, fmt.Errorf(
			"llmtest: only %d of %d judges succeeded (minimum %d required): %w",
			len(scores), opts.Judges, opts.MinJudges, errors.Join(errs...),
		)
	}

	return &Result{
		Scores: scores,
		Notes:  notes,
		Errors: errs,
		Stats:  computeStats(scores),
	}, nil
}
