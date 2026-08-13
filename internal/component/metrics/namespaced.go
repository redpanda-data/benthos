// Copyright 2025 Redpanda Data, Inc.

package metrics

import (
	"maps"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Namespaced wraps a child metrics exporter and exposes a Type API that
// adds namespacing labels and name prefixes to new.
type Namespaced struct {
	labels   map[string]string
	mappings []*Mapping
	child    Type
	cleanup  *namespacedCleanup
}

// NewNamespaced wraps a metrics exporter and adds prefixes and custom labels.
func NewNamespaced(child Type) *Namespaced {
	return &Namespaced{
		child: child,
	}
}

// Noop returns a namespaced metrics aggregator with a noop child.
func Noop() *Namespaced {
	return &Namespaced{
		child: DudType{},
	}
}

// WithStats returns a namespaced metrics exporter with a different stats
// implementation.
func (n *Namespaced) WithStats(s Type) *Namespaced {
	newNs := *n
	newNs.child = s
	return &newNs
}

// WithCleanup returns a metrics exporter scope that removes all series created
// through it when closed, where supported by the underlying exporter.
func (n *Namespaced) WithCleanup() *Namespaced {
	newNs := *n
	newNs.cleanup = &namespacedCleanup{
		stats: map[string]StatDeleter{},
	}
	return &newNs
}

// WithLabels returns a namespaced metrics exporter with a new set of labels,
// which are added to any prior labels.
func (n *Namespaced) WithLabels(labels ...string) *Namespaced {
	newLabels := map[string]string{}
	maps.Copy(newLabels, n.labels)
	for i := 0; i < len(labels)-1; i += 2 {
		newLabels[labels[i]] = labels[i+1]
	}
	newNs := *n
	newNs.labels = newLabels
	return &newNs
}

// WithMapping returns a namespaced metrics exporter with a new mapping.
// Mappings are applied _before_ the prefix and static labels are applied.
// Mappings already added are executed after this new mapping.
func (n *Namespaced) WithMapping(m *Mapping) *Namespaced {
	newNs := *n
	newMappings := make([]*Mapping, 0, len(n.mappings)+1)
	newMappings = append(newMappings, m)
	newMappings = append(newMappings, n.mappings...)
	newNs.mappings = newMappings
	return &newNs
}

//------------------------------------------------------------------------------

// Child returns the underlying metrics type.
func (n *Namespaced) Child() Type {
	return n.child
}

// HandlerFunc returns the http handler of the child.
func (n *Namespaced) HandlerFunc() http.HandlerFunc {
	return n.child.HandlerFunc()
}

//------------------------------------------------------------------------------

func (n *Namespaced) getPathAndLabels(path string) (newPath string, labelKeys, labelValues []string) {
	newPath = path
	if len(n.labels) > 0 {
		labelKeys = make([]string, 0, len(n.labels))
		for k := range n.labels {
			labelKeys = append(labelKeys, k)
		}
		sort.Strings(labelKeys)
		labelValues = make([]string, 0, len(n.labels))
		for _, k := range labelKeys {
			labelValues = append(labelValues, n.labels[k])
		}
	}
	for _, mapping := range n.mappings {
		if newPath, labelKeys, labelValues = mapping.mapPath(newPath, labelKeys, labelValues); newPath == "" {
			return
		}
	}
	return
}

type counterVecWithStatic struct {
	staticValues []string
	child        StatCounterVec
	path         string
	cleanup      *namespacedCleanup
}

func (c *counterVecWithStatic) With(values ...string) StatCounter {
	newValues := make([]string, 0, len(c.staticValues)+len(values))
	newValues = append(newValues, c.staticValues...)
	newValues = append(newValues, values...)
	stat := c.child.With(newValues...)
	if c.cleanup != nil {
		c.cleanup.track(cleanupKey("counter", c.path, newValues), stat)
	}
	return stat
}

type timerVecWithStatic struct {
	staticValues []string
	child        StatTimerVec
	path         string
	cleanup      *namespacedCleanup
}

func (c *timerVecWithStatic) With(values ...string) StatTimer {
	newValues := make([]string, 0, len(c.staticValues)+len(values))
	newValues = append(newValues, c.staticValues...)
	newValues = append(newValues, values...)
	stat := c.child.With(newValues...)
	if c.cleanup != nil {
		c.cleanup.track(cleanupKey("timer", c.path, newValues), stat)
	}
	return stat
}

type gaugeVecWithStatic struct {
	staticValues []string
	child        StatGaugeVec
	path         string
	cleanup      *namespacedCleanup
}

func (c *gaugeVecWithStatic) With(values ...string) StatGauge {
	newValues := make([]string, 0, len(c.staticValues)+len(values))
	newValues = append(newValues, c.staticValues...)
	newValues = append(newValues, values...)
	stat := c.child.With(newValues...)
	if c.cleanup != nil {
		c.cleanup.track(cleanupKey("gauge", c.path, newValues), stat)
	}
	return stat
}

//------------------------------------------------------------------------------

type namespacedCleanup struct {
	mut    sync.Mutex
	stats  map[string]StatDeleter
	closed bool
}

func cleanupKey(kind, path string, labelValues []string) string {
	var b strings.Builder
	writePart := func(value string) {
		b.WriteString(strconv.Itoa(len(value)))
		b.WriteByte(':')
		b.WriteString(value)
	}
	writePart(kind)
	writePart(path)
	for _, value := range labelValues {
		writePart(value)
	}
	return b.String()
}

func (c *namespacedCleanup) track(key string, stat any) {
	deleter, ok := stat.(StatDeleter)
	if !ok {
		return
	}
	c.mut.Lock()
	if c.closed {
		c.mut.Unlock()
		deleter.Delete()
		return
	}
	if _, exists := c.stats[key]; !exists {
		c.stats[key] = deleter
	}
	c.mut.Unlock()
}

func (c *namespacedCleanup) close() {
	c.mut.Lock()
	if c.closed {
		c.mut.Unlock()
		return
	}
	c.closed = true
	stats := c.stats
	c.stats = nil
	c.mut.Unlock()

	for _, stat := range stats {
		stat.Delete()
	}
}

func (n *Namespaced) track(kind, path string, labelValues []string, stat any) {
	if n.cleanup != nil {
		n.cleanup.track(cleanupKey(kind, path, labelValues), stat)
	}
}

// GetCounter returns an editable counter stat for a given path.
func (n *Namespaced) GetCounter(path string) StatCounter {
	path, labelKeys, labelValues := n.getPathAndLabels(path)
	if path == "" {
		return DudStat{}
	}
	if len(labelKeys) > 0 {
		stat := n.child.GetCounterVec(path, labelKeys...).With(labelValues...)
		n.track("counter", path, labelValues, stat)
		return stat
	}
	stat := n.child.GetCounter(path)
	n.track("counter", path, nil, stat)
	return stat
}

// GetCounterVec returns an editable counter stat for a given path with labels,
// these labels must be consistent with any other metrics registered on the same
// path.
func (n *Namespaced) GetCounterVec(path string, labelNames ...string) StatCounterVec {
	path, staticKeys, staticValues := n.getPathAndLabels(path)
	if path == "" {
		return FakeCounterVec(func(...string) StatCounter {
			return DudStat{}
		})
	}
	if len(staticKeys) > 0 {
		newNames := make([]string, 0, len(staticKeys)+len(labelNames))
		newNames = append(newNames, staticKeys...)
		newNames = append(newNames, labelNames...)
		return &counterVecWithStatic{
			staticValues: staticValues,
			child:        n.child.GetCounterVec(path, newNames...),
			path:         path,
			cleanup:      n.cleanup,
		}
	}
	child := n.child.GetCounterVec(path, labelNames...)
	if n.cleanup == nil {
		return child
	}
	return &counterVecWithStatic{
		child:   child,
		path:    path,
		cleanup: n.cleanup,
	}
}

// GetTimer returns an editable timer stat for a given path.
func (n *Namespaced) GetTimer(path string) StatTimer {
	path, labelKeys, labelValues := n.getPathAndLabels(path)
	if path == "" {
		return DudStat{}
	}
	if len(labelKeys) > 0 {
		stat := n.child.GetTimerVec(path, labelKeys...).With(labelValues...)
		n.track("timer", path, labelValues, stat)
		return stat
	}
	stat := n.child.GetTimer(path)
	n.track("timer", path, nil, stat)
	return stat
}

// GetTimerVec returns an editable timer stat for a given path with labels,
// these labels must be consistent with any other metrics registered on the same
// path.
func (n *Namespaced) GetTimerVec(path string, labelNames ...string) StatTimerVec {
	path, staticKeys, staticValues := n.getPathAndLabels(path)
	if path == "" {
		return FakeTimerVec(func(...string) StatTimer {
			return DudStat{}
		})
	}
	if len(staticKeys) > 0 {
		newNames := make([]string, 0, len(staticKeys)+len(labelNames))
		newNames = append(newNames, staticKeys...)
		newNames = append(newNames, labelNames...)
		return &timerVecWithStatic{
			staticValues: staticValues,
			child:        n.child.GetTimerVec(path, newNames...),
			path:         path,
			cleanup:      n.cleanup,
		}
	}
	child := n.child.GetTimerVec(path, labelNames...)
	if n.cleanup == nil {
		return child
	}
	return &timerVecWithStatic{
		child:   child,
		path:    path,
		cleanup: n.cleanup,
	}
}

// GetGauge returns an editable gauge stat for a given path.
func (n *Namespaced) GetGauge(path string) StatGauge {
	path, labelKeys, labelValues := n.getPathAndLabels(path)
	if path == "" {
		return DudStat{}
	}
	if len(labelKeys) > 0 {
		stat := n.child.GetGaugeVec(path, labelKeys...).With(labelValues...)
		n.track("gauge", path, labelValues, stat)
		return stat
	}
	stat := n.child.GetGauge(path)
	n.track("gauge", path, nil, stat)
	return stat
}

// GetGaugeVec returns an editable gauge stat for a given path with labels,
// these labels must be consistent with any other metrics registered on the same
// path.
func (n *Namespaced) GetGaugeVec(path string, labelNames ...string) StatGaugeVec {
	path, staticKeys, staticValues := n.getPathAndLabels(path)
	if path == "" {
		return FakeGaugeVec(func(...string) StatGauge {
			return DudStat{}
		})
	}
	if len(staticKeys) > 0 {
		newNames := make([]string, 0, len(staticKeys)+len(labelNames))
		newNames = append(newNames, staticKeys...)
		newNames = append(newNames, labelNames...)
		return &gaugeVecWithStatic{
			staticValues: staticValues,
			child:        n.child.GetGaugeVec(path, newNames...),
			path:         path,
			cleanup:      n.cleanup,
		}
	}
	child := n.child.GetGaugeVec(path, labelNames...)
	if n.cleanup == nil {
		return child
	}
	return &gaugeVecWithStatic{
		child:   child,
		path:    path,
		cleanup: n.cleanup,
	}
}

// Close stops aggregating stats and cleans up resources.
func (n *Namespaced) Close() error {
	if n.cleanup != nil {
		n.cleanup.close()
		return nil
	}
	return n.child.Close()
}
