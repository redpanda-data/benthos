package agentexam

import (
	"context"
	"testing"
)

// T runs an exam as a Go test. Each result becomes a subtest under the exam
// name. Results with a Score below 1 are reported as errors.
func T(t *testing.T, exam *Exam, opts *Options) {
	t.Helper()

	ctx := context.Background()
	if dl, ok := t.Deadline(); ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, dl)
		defer cancel()
	}

	results, err := Run(ctx, exam, opts)
	if err != nil {
		t.Fatalf("exam %s: %v", exam.Name, err)
	}

	for _, r := range results {
		t.Run(r.ID, func(t *testing.T) {
			t.Helper()
			if r.Score < 1 {
				t.Errorf("%s: score %.2f: %s", r.Name, r.Score, r.Error)
			}
		})
	}
}
