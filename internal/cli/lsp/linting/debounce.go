package linting

import (
	"sync"
	"time"

	"github.com/redpanda-data/benthos/v4/internal/cli/common"
	"github.com/redpanda-data/benthos/v4/internal/docs"
	"github.com/redpanda-data/benthos/v4/public/bloblang"
)

type Debouncer struct {
	Cfg    docs.LintConfig
	mu     sync.Mutex
	timers map[string]*time.Timer
}

func NewDebouncer(opts *common.CLIOpts) *Debouncer {
	lintConf := docs.NewLintConfig(opts.Environment)
	lintConf.BloblangEnv = bloblang.XWrapEnvironment(opts.BloblEnvironment)

	return &Debouncer{
		timers: make(map[string]*time.Timer),
		Cfg:    lintConf,
	}
}

func (d *Debouncer) Debounce(uri string, delay time.Duration, lintAction func()) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// If a timer already exists for this URI, stop it
	if timer, ok := d.timers[uri]; ok {
		timer.Stop()
	}

	// Create a new timer
	d.timers[uri] = time.AfterFunc(delay, func() {
		lintAction()
		d.mu.Lock()
		delete(d.timers, uri)
		d.mu.Unlock()
	})
}
