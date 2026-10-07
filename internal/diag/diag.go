// Package diag collects the warnings of one generate run (issue #229, step S5).
//
// Rendering a project raises advice that must be shown once however many presets
// raise it, and one summary of the rules whose activation a tool cannot express.
// That bookkeeping used to live in package-level variables, which made two runs in
// one process reset and flush each other's entries. A Collector is the same
// bookkeeping owned by one run: it hangs off the loaded config (Config.Diag), so
// runs on different projects share nothing and need no lock between them.
//
// A nil *Collector stands for the process default collector, which serves a
// caller that renders without a generate run (one file, a probe, a test). Its
// destination is SetDefaultSink, the CLI's logger unless replaced.
package diag

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

// Sink receives a warning.
type Sink func(msg string, args ...any)

var (
	defaultMu   sync.RWMutex
	defaultSink Sink = logger.Std().Warn
	// processDefault is the collector a nil *Collector stands for.
	processDefault = New(nil)
)

// SetDefaultSink replaces the destination of collectors created without a sink
// (and of the process default collector) and returns a function restoring the
// previous one. It forgets what the default collector already issued, so a test
// sees each message again.
func SetDefaultSink(fn Sink) (restore func()) {
	defaultMu.Lock()
	prev := defaultSink
	defaultSink = fn
	defaultMu.Unlock()
	processDefault.forget()
	return func() {
		defaultMu.Lock()
		defaultSink = prev
		defaultMu.Unlock()
	}
}

func loadDefaultSink() Sink {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return defaultSink
}

// Collector is the warning state of one generate run. It is safe for concurrent
// use, since presets of one run may render in parallel.
type Collector struct {
	mu         sync.Mutex
	sink       Sink
	downgrades map[string]struct{}
	// seen holds the keys already issued since the last Reset, by namespace.
	seen map[string]struct{}
	// sticky holds keys that are said once for the life of the collector: Reset
	// and SetSink leave them alone, so a command that renders twice (a clean plan
	// and then the clean) does not repeat an advice that is about the project.
	sticky map[string]struct{}
}

// New returns a Collector that issues warnings through sink; a nil sink uses the
// default sink.
func New(sink Sink) *Collector {
	return &Collector{sink: sink, downgrades: map[string]struct{}{}, seen: map[string]struct{}{}, sticky: map[string]struct{}{}}
}

// self resolves a nil receiver to the process default collector.
func (c *Collector) self() *Collector {
	if c == nil {
		return processDefault
	}
	return c
}

func (c *Collector) current() Sink {
	c.mu.Lock()
	sink := c.sink
	c.mu.Unlock()
	if sink != nil {
		return sink
	}
	return loadDefaultSink()
}

func (c *Collector) forget() {
	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.seen)
}

// Warn issues msg once per run: a message already issued since the last Reset is
// not repeated.
func (c *Collector) Warn(msg string, args ...any) {
	c = c.self()
	if c.first("msg", msg) {
		c.current()(msg, args...)
	}
}

// Raise issues msg every time, with no de-duplication.
func (c *Collector) Raise(msg string, args ...any) {
	c.self().current()(msg, args...)
}

// Once reports whether this is the first call for key in namespace since the last Reset.
func (c *Collector) Once(namespace, key string) bool {
	return c.self().first(namespace, key)
}

// Sticky reports whether this is the first call for key in the life of the
// collector; unlike Once, Reset does not forget it.
func (c *Collector) Sticky(key string) bool {
	c = c.self()
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.sticky[key]; ok {
		return false
	}
	c.sticky[key] = struct{}{}
	return true
}

// OnceWarn issues msg when this is the first call for key in namespace.
func (c *Collector) OnceWarn(namespace, key, msg string, args ...any) {
	if c.Once(namespace, key) {
		c.Raise(msg, args...)
	}
}

func (c *Collector) first(namespace, key string) bool {
	k := namespace + "\x00" + key
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.seen[k]; ok {
		return false
	}
	c.seen[k] = struct{}{}
	return true
}

// RecordDowngrade notes that an item's activation mode could not be expressed by
// the target and was rendered as always-on. Duplicates (the same item rendered
// into several root files) collapse into one entry.
func (c *Collector) RecordDowngrade(kind, name, mode string) {
	logger.Std().Debug("Activation downgraded to always-on in inline output", "kind", kind, "name", name, "mode", mode)
	c = c.self()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.downgrades[kind+" "+name] = struct{}{}
}

// Reset discards the downgrades and the de-duplication state; call it when a run starts.
func (c *Collector) Reset() {
	c = c.self()
	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.downgrades)
	clear(c.seen)
}

// Flush issues one warning listing every recorded downgrade, sorted, then clears
// them. It issues nothing when none were recorded.
func (c *Collector) Flush() {
	c = c.self()
	c.mu.Lock()
	entries := make([]string, 0, len(c.downgrades))
	for k := range c.downgrades {
		entries = append(entries, k)
	}
	clear(c.downgrades)
	c.mu.Unlock()
	if len(entries) == 0 {
		return
	}
	sort.Strings(entries)
	c.current()(strconv.Itoa(len(entries))+" rules/context items use an activation the target tool cannot express; "+
		"they are loaded always:", "items", strings.Join(entries, ", "))
}

// SetSink replaces the sink and returns a function restoring the previous one.
func (c *Collector) SetSink(sink Sink) (restore func()) {
	c = c.self()
	c.mu.Lock()
	prev := c.sink
	c.sink = sink
	clear(c.seen)
	c.mu.Unlock()
	return func() {
		c.mu.Lock()
		c.sink = prev
		c.mu.Unlock()
	}
}

// Silence drops the warnings issued until the returned function runs, and keeps
// them out of the "already issued" set, so a later real render still shows each
// one. It is for a caller that renders a document only to learn whether it holds
// anything.
func (c *Collector) Silence() (restore func()) {
	c = c.self()
	c.mu.Lock()
	prevSink := c.sink
	prevSeen := make(map[string]struct{}, len(c.seen))
	for k := range c.seen {
		prevSeen[k] = struct{}{}
	}
	c.sink = func(string, ...any) {}
	c.mu.Unlock()
	return func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.sink = prevSink
		clear(c.seen)
		for k := range prevSeen {
			c.seen[k] = struct{}{}
		}
	}
}

// Default is the process default collector, the one a nil *Collector stands for.
func Default() *Collector { return processDefault }

type ctxKey struct{}

// WithContext returns ctx carrying c, for call chains that pass a context but no
// config (the includes resolvers). A nil c leaves ctx unchanged.
func WithContext(ctx context.Context, c *Collector) context.Context {
	if c == nil {
		return ctx
	}
	return context.WithValue(ctx, ctxKey{}, c)
}

// FromContext returns the Collector ctx carries; without one it is nil, which
// stands for the process default collector.
func FromContext(ctx context.Context) *Collector {
	if ctx != nil {
		if c, ok := ctx.Value(ctxKey{}).(*Collector); ok {
			return c
		}
	}
	return nil
}
