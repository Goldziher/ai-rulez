// Package watch re-runs a task when files change. The Scheduler coalesces change
// notifications into debounced, never-overlapping runs; the Watcher turns
// fsnotify events on a set of targets into those notifications.
package watch

import (
	"context"
	"sort"
	"sync"
	"time"
)

// DefaultDebounce is the quiet period after the last change before a run starts.
// Editor save storms and `git checkout` emit many events within milliseconds;
// coalescing them keeps the terminal readable and avoids redundant work.
const DefaultDebounce = 300 * time.Millisecond

// Timer is a pending callback that can be cancelled.
type Timer interface {
	Stop() bool
}

// Clock schedules callbacks. It is injected so the Scheduler can be tested
// without real timing.
type Clock interface {
	AfterFunc(d time.Duration, f func()) Timer
}

// RealClock is the Clock backed by the time package; f runs on its own goroutine.
type RealClock struct{}

// AfterFunc implements Clock.
func (RealClock) AfterFunc(d time.Duration, f func()) Timer { return time.AfterFunc(d, f) }

// RunFunc performs one run for the paths that changed since the previous one.
// An error is reported through the Scheduler's OnError and never stops watching.
type RunFunc func(ctx context.Context, triggers []string) error

// Scheduler coalesces notifications into debounced runs.
//
// Guarantees: at most one run is in flight; every notified path reaches exactly
// one run; notifications that arrive during a run schedule one follow-up run
// after it finishes, so a change is neither lost nor multiplied per event.
type Scheduler struct {
	run      RunFunc
	onError  func(err error, triggers []string)
	debounce time.Duration
	clock    Clock

	ctx    context.Context
	cancel context.CancelFunc

	mu      sync.Mutex
	pending map[string]struct{}
	timer   Timer
	gen     uint64
	running bool
	closed  bool
	idle    *sync.Cond
}

// NewScheduler builds a Scheduler. A zero debounce means DefaultDebounce and a
// nil clock means RealClock. onError may be nil.
func NewScheduler(run RunFunc, onError func(err error, triggers []string), debounce time.Duration, clock Clock) *Scheduler {
	if debounce <= 0 {
		debounce = DefaultDebounce
	}
	if clock == nil {
		clock = RealClock{}
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Scheduler{
		run:      run,
		onError:  onError,
		debounce: debounce,
		clock:    clock,
		ctx:      ctx,
		cancel:   cancel,
		pending:  map[string]struct{}{},
	}
	s.idle = sync.NewCond(&s.mu)
	return s
}

// Notify records a changed path and (re)starts the debounce timer.
func (s *Scheduler) Notify(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.pending[path] = struct{}{}
	s.armLocked(s.debounce)
}

// Trigger records a path and runs without waiting for the debounce, for the
// initial run.
func (s *Scheduler) Trigger(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.pending[path] = struct{}{}
	s.armLocked(0)
}

// Close drops pending changes, cancels the context handed to the run and waits
// for an in-flight run to return. It must not be called from inside a run.
func (s *Scheduler) Close() {
	s.mu.Lock()
	s.closed = true
	s.disarmLocked()
	s.pending = map[string]struct{}{}
	s.cancel()
	for s.running {
		s.idle.Wait()
	}
	s.mu.Unlock()
}

func (s *Scheduler) disarmLocked() {
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
}

func (s *Scheduler) armLocked(d time.Duration) {
	s.disarmLocked()
	s.gen++
	gen := s.gen
	s.timer = s.clock.AfterFunc(d, func() { s.fire(gen) })
}

func (s *Scheduler) fire(gen uint64) {
	s.mu.Lock()
	// A stale callback: its timer was replaced after the callback was queued.
	if s.closed || gen != s.gen {
		s.mu.Unlock()
		return
	}
	s.timer = nil
	if s.running || len(s.pending) == 0 {
		// The finishing run re-arms the timer when changes are pending.
		s.mu.Unlock()
		return
	}
	triggers := make([]string, 0, len(s.pending))
	for p := range s.pending {
		triggers = append(triggers, p)
	}
	sort.Strings(triggers)
	s.pending = map[string]struct{}{}
	s.running = true
	s.mu.Unlock()

	err := s.run(s.ctx, triggers)
	if err != nil && s.onError != nil {
		s.onError(err, triggers)
	}

	s.mu.Lock()
	s.running = false
	if !s.closed && len(s.pending) > 0 {
		s.armLocked(s.debounce)
	}
	s.idle.Broadcast()
	s.mu.Unlock()
}
