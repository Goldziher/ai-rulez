package watch

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"
)

// fakeClock runs due callbacks synchronously from Advance.
type fakeClock struct {
	mu     sync.Mutex
	now    time.Duration
	timers []*fakeTimer
}

type fakeTimer struct {
	at      time.Duration
	f       func()
	stopped bool
	fired   bool
}

func (t *fakeTimer) Stop() bool {
	was := !t.stopped && !t.fired
	t.stopped = true
	return was
}

func (c *fakeClock) AfterFunc(d time.Duration, f func()) Timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &fakeTimer{at: c.now + d, f: f}
	c.timers = append(c.timers, t)
	return t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now += d
	var due []*fakeTimer
	for _, t := range c.timers {
		if !t.stopped && !t.fired && t.at <= c.now {
			t.fired = true
			due = append(due, t)
		}
	}
	c.mu.Unlock()
	for _, t := range due {
		t.f()
	}
}

func TestScheduler(t *testing.T) {
	const d = 300 * time.Millisecond

	t.Run("coalesces a burst into one run after the debounce", func(t *testing.T) {
		// Arrange
		clock := &fakeClock{}
		var runs [][]string
		s := NewScheduler(func(_ context.Context, tr []string) error { runs = append(runs, tr); return nil }, nil, d, clock)
		defer s.Close()

		// Act
		s.Notify("b")
		clock.Advance(200 * time.Millisecond)
		s.Notify("a")
		s.Notify("b")
		clock.Advance(200 * time.Millisecond) // 200ms after the last notify: not yet
		early := len(runs)
		clock.Advance(100 * time.Millisecond)

		// Assert
		if early != 0 {
			t.Fatalf("ran before the debounce elapsed: %v", runs)
		}
		if want := [][]string{{"a", "b"}}; !reflect.DeepEqual(runs, want) {
			t.Fatalf("runs = %v, want %v", runs, want)
		}
	})

	t.Run("changes during a run cause exactly one follow-up run and never overlap", func(t *testing.T) {
		clock := &fakeClock{}
		var runs [][]string
		active, maxActive := 0, 0
		var s *Scheduler
		s = NewScheduler(func(_ context.Context, tr []string) error {
			active++
			maxActive = max(maxActive, active)
			runs = append(runs, tr)
			if len(runs) == 1 {
				// A burst arrives while the first run is in flight.
				s.Notify("x")
				s.Notify("y")
				s.Notify("x")
				clock.Advance(d) // the debounce elapses mid-run: must not start a second run
			}
			active--
			return nil
		}, nil, d, clock)
		defer s.Close()

		s.Notify("first")
		clock.Advance(d)
		if len(runs) != 1 {
			t.Fatalf("runs after first = %v", runs)
		}
		clock.Advance(d)

		if want := [][]string{{"first"}, {"x", "y"}}; !reflect.DeepEqual(runs, want) {
			t.Fatalf("runs = %v, want %v", runs, want)
		}
		if maxActive != 1 {
			t.Fatalf("runs overlapped: max concurrency %d", maxActive)
		}
		clock.Advance(10 * d)
		if len(runs) != 2 {
			t.Fatalf("extra run happened: %v", runs)
		}
	})

	t.Run("a failing run is reported and watching continues", func(t *testing.T) {
		clock := &fakeClock{}
		var errs []error
		calls := 0
		s := NewScheduler(func(context.Context, []string) error {
			calls++
			if calls == 1 {
				return errors.New("bad config")
			}
			return nil
		}, func(err error, _ []string) { errs = append(errs, err) }, d, clock)
		defer s.Close()

		s.Notify("a")
		clock.Advance(d)
		s.Notify("b")
		clock.Advance(d)

		if len(errs) != 1 || calls != 2 {
			t.Fatalf("errs=%v calls=%d, want 1 error and 2 calls", errs, calls)
		}
	})

	t.Run("trigger runs without waiting for the debounce", func(t *testing.T) {
		clock := &fakeClock{}
		var runs [][]string
		s := NewScheduler(func(_ context.Context, tr []string) error { runs = append(runs, tr); return nil }, nil, d, clock)
		defer s.Close()

		s.Trigger("initial")
		clock.Advance(0)

		if want := [][]string{{"initial"}}; !reflect.DeepEqual(runs, want) {
			t.Fatalf("runs = %v, want %v", runs, want)
		}
	})

	t.Run("close drops pending changes and ignores later notifications", func(t *testing.T) {
		clock := &fakeClock{}
		calls := 0
		s := NewScheduler(func(context.Context, []string) error { calls++; return nil }, nil, d, clock)

		s.Notify("a")
		s.Close()
		s.Notify("b")
		clock.Advance(10 * d)

		if calls != 0 {
			t.Fatalf("ran %d times after Close", calls)
		}
	})

	t.Run("close cancels the run context and waits for the run", func(t *testing.T) {
		started := make(chan struct{})
		finished := make(chan struct{})
		s := NewScheduler(func(ctx context.Context, _ []string) error {
			close(started)
			<-ctx.Done()
			close(finished)
			return nil
		}, nil, d, RealClock{})

		s.Trigger("initial")
		<-started
		s.Close()

		select {
		case <-finished:
		default:
			t.Fatal("Close returned before the run finished")
		}
	})
}
