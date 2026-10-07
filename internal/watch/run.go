package watch

import (
	"context"
	"path/filepath"
	"strings"
	"time"
)

// Options configures Watch.
type Options struct {
	// Targets lists what to watch. It is called before the first run and after
	// every run, so sources a run discovers (a new local include) are picked up.
	Targets func() []Target
	// Ignore filters event paths; DefaultIgnore is always applied to the part of
	// a path below its target root.
	Ignore func(path string) bool
	// Run is the work to repeat. It always runs once at start with the trigger
	// "initial".
	Run RunFunc
	// OnError receives a run's error; watching continues.
	OnError  func(err error, triggers []string)
	Debounce time.Duration
	Clock    Clock
	Log      Logf
	// Warn receives problems the user can act on (a watch limit reached). It
	// defaults to Log.
	Warn Logf
}

// Watch runs Options.Run once, then again whenever a watched path changes, until
// ctx is canceled. The watchers are armed before the first run so a change made
// during it is not lost. It waits for an in-flight run before returning.
func Watch(ctx context.Context, o Options) error {
	var sched *Scheduler
	w, err := NewWatcher(o.Ignore, func(p string) { sched.Notify(p) }, o.Log)
	if err != nil {
		return err
	}
	defer func() { _ = w.Close() }()
	w.SetWarn(o.Warn)

	addTargets := func() {
		if o.Targets == nil {
			return
		}
		w.Sync(o.Targets())
	}
	run := func(ctx context.Context, triggers []string) error {
		defer addTargets()
		return o.Run(ctx, triggers)
	}
	sched = NewScheduler(run, o.OnError, o.Debounce, o.Clock)
	addTargets()
	sched.Trigger("initial")

	w.Run(ctx)
	sched.Close()
	return nil
}

// DefaultIgnore reports editor swap/backup files, VCS internals and OS litter,
// which must never trigger a run. path should be relative to the watched root:
// every component is checked, so an absolute path would be ignored whenever the
// project itself sits below a directory named .git or node_modules.
func DefaultIgnore(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == ".git" || part == "node_modules" {
			return true
		}
	}
	base := filepath.Base(path)
	switch {
	case base == ".DS_Store", base == "4913", base == "Thumbs.db":
		return true
	case strings.HasPrefix(base, ".#"), strings.HasPrefix(base, "._"):
		return true
	case strings.HasPrefix(base, "#") && strings.HasSuffix(base, "#"):
		return true
	}
	for _, suffix := range []string{"~", ".swp", ".swx", ".swo", ".tmp", ".crswap", ".kate-swp"} {
		if strings.HasSuffix(base, suffix) {
			return true
		}
	}
	return false
}
