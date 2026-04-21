package translator

// recorder accumulates Changes and per-node classifications during a single
// Migrate call. Translation rules call its Record* methods as they visit V1
// AST nodes; the final Coverage is computed from the counts.
//
// The recorder is not safe for concurrent use; Migrate is single-threaded by
// design.
type recorder struct {
	opts     Options
	changes  []Change
	coverage Coverage
	// warningsAsErrors mirrors opts.TreatWarningsAsErrors; a separate field
	// keeps the hot path branch-free.
	warningsAsErrors bool
}

// newRecorder constructs a recorder from options.
func newRecorder(opts Options) *recorder {
	return &recorder{
		opts:             opts,
		warningsAsErrors: opts.TreatWarningsAsErrors,
	}
}

// Exact increments the Translated counter: a V1 node mapped 1:1 to V2 with no
// semantic divergence. No Change is recorded.
func (r *recorder) Exact() {
	r.coverage.Total++
	r.coverage.Translated++
}

// Rewritten increments the Rewritten counter and records a Change. Use this
// when the translator chose V2 semantics that differ from V1, or when the
// rewrite is a pure idiom (Info-level) that the caller may want to note.
func (r *recorder) Rewritten(ch Change) {
	r.coverage.Total++
	r.coverage.Rewritten++
	r.emit(ch)
}

// Unsupported increments the Unsupported counter and records an Error Change.
// Use when the V1 construct has no V2 equivalent and the translator emits a
// MIGRATION comment in place.
func (r *recorder) Unsupported(ch Change) {
	r.coverage.Total++
	r.coverage.Unsupported++
	ch.Severity = SeverityError
	ch.Category = CategoryUnsupported
	r.emit(ch)
}

// emit writes the Change to the report, respecting verbose and warnings-as-
// errors options.
func (r *recorder) emit(ch Change) {
	if r.warningsAsErrors && ch.Severity == SeverityWarning {
		ch.Severity = SeverityError
	}
	if ch.Severity == SeverityInfo && !r.opts.Verbose {
		return
	}
	r.changes = append(r.changes, ch)
}

// finalise computes the final Coverage ratio and returns the Report.
func (r *recorder) finalise(v2 string) *Report {
	r.coverage.Ratio = computeRatio(r.coverage)
	return &Report{
		V2Mapping: v2,
		Changes:   r.changes,
		Coverage:  r.coverage,
	}
}
