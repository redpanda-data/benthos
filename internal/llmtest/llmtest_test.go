package llmtest

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockProvider struct {
	responses []*JudgeResponse
	err       error
	calls     atomic.Int32
}

func (m *mockProvider) Judge(_ context.Context, _ JudgeRequest) (*JudgeResponse, error) {
	idx := int(m.calls.Add(1)) - 1
	if m.err != nil {
		return nil, m.err
	}
	if idx < len(m.responses) {
		return m.responses[idx], nil
	}
	return m.responses[0], nil
}

func TestEval_SingleJudge(t *testing.T) {
	p := &mockProvider{
		responses: []*JudgeResponse{
			{Score: 85, Notes: []string{"looks good"}},
		},
	}

	result, err := Eval(context.Background(), p, EvalOptions{
		Prompt: "test prompt",
	})
	require.NoError(t, err)

	assert.Equal(t, []int{85}, result.Scores)
	assert.Equal(t, [][]string{{"looks good"}}, result.Notes)
	assert.Empty(t, result.Errors)
	assert.Equal(t, 85.0, result.Stats.Mean)
}

func TestEval_MultipleJudges(t *testing.T) {
	p := &mockProvider{
		responses: []*JudgeResponse{
			{Score: 70, Notes: []string{"decent"}},
			{Score: 80, Notes: []string{"good"}},
			{Score: 90, Notes: []string{"great"}},
		},
	}

	result, err := Eval(context.Background(), p, EvalOptions{
		Prompt: "test prompt",
		Judges: 3,
	})
	require.NoError(t, err)

	assert.Len(t, result.Scores, 3)
	assert.Len(t, result.Notes, 3)
	assert.Equal(t, 80.0, result.Stats.Mean)
	assert.Equal(t, 80.0, result.Stats.Median)
	assert.Equal(t, 70, result.Stats.Min)
	assert.Equal(t, 90, result.Stats.Max)
}

func TestEval_EmptyPrompt(t *testing.T) {
	_, err := Eval(context.Background(), &mockProvider{}, EvalOptions{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "prompt is required")
}

func TestEval_MinJudgesExceedsJudges(t *testing.T) {
	_, err := Eval(context.Background(), &mockProvider{}, EvalOptions{
		Prompt:    "test",
		Judges:    2,
		MinJudges: 5,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "MinJudges")
}

func TestEval_AllJudgesFail(t *testing.T) {
	p := &mockProvider{err: errors.New("connection refused")}

	_, err := Eval(context.Background(), p, EvalOptions{
		Prompt: "test prompt",
		Judges: 3,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "only 0 of 3 judges succeeded")
}

func TestEval_PartialFailureWithTolerance(t *testing.T) {
	callCount := atomic.Int32{}
	p := &failNthProvider{
		failIndex: 1,
		response:  &JudgeResponse{Score: 80, Notes: []string{"ok"}},
		calls:     &callCount,
	}

	result, err := Eval(context.Background(), p, EvalOptions{
		Prompt:    "test prompt",
		Judges:    3,
		MinJudges: 2,
	})
	require.NoError(t, err)

	assert.Len(t, result.Scores, 2)
	assert.Len(t, result.Errors, 1)
	assert.Equal(t, 80.0, result.Stats.Mean)
}

func TestEval_PartialFailureWithoutTolerance(t *testing.T) {
	callCount := atomic.Int32{}
	p := &failNthProvider{
		failIndex: 0,
		response:  &JudgeResponse{Score: 80, Notes: []string{"ok"}},
		calls:     &callCount,
	}

	_, err := Eval(context.Background(), p, EvalOptions{
		Prompt: "test prompt",
		Judges: 3,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "only 2 of 3 judges succeeded")
}

func TestEval_DefaultsJudgesToOne(t *testing.T) {
	p := &mockProvider{
		responses: []*JudgeResponse{
			{Score: 50, Notes: []string{"meh"}},
		},
	}

	result, err := Eval(context.Background(), p, EvalOptions{
		Prompt: "test prompt",
		Judges: 0,
	})
	require.NoError(t, err)
	assert.Len(t, result.Scores, 1)
}

// failNthProvider fails the Nth call (0-indexed) and succeeds all others.
type failNthProvider struct {
	failIndex int
	response  *JudgeResponse
	calls     *atomic.Int32
}

func (f *failNthProvider) Judge(_ context.Context, _ JudgeRequest) (*JudgeResponse, error) {
	idx := int(f.calls.Add(1)) - 1
	if idx == f.failIndex {
		return nil, errors.New("simulated failure")
	}
	return f.response, nil
}
