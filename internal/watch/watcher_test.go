package watch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

const eventTimeout = 10 * time.Second

// collector gathers notified paths and lets a test wait for one.
type collector struct {
	mu    sync.Mutex
	paths []string
	ch    chan struct{}
}

func newCollector() *collector { return &collector{ch: make(chan struct{}, 1024)} }

func (c *collector) notify(p string) {
	c.mu.Lock()
	c.paths = append(c.paths, p)
	c.mu.Unlock()
	c.ch <- struct{}{}
}

func (c *collector) seen(p string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, got := range c.paths {
		if got == p {
			return true
		}
	}
	return false
}

func (c *collector) seenUnder(dir string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, got := range c.paths {
		if got == dir || strings.HasPrefix(got, dir+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// waitFor polls (with a generous timeout) until a path was notified.
func (c *collector) waitFor(t *testing.T, p string) {
	t.Helper()
	deadline := time.After(eventTimeout)
	for !c.seen(p) {
		select {
		case <-c.ch:
		case <-deadline:
			t.Fatalf("no event for %s; saw %v", p, c.paths)
		}
	}
}

func startWatcher(t *testing.T, ignore func(string) bool, c *collector, targets ...Target) *Watcher {
	t.Helper()
	w, err := NewWatcher(ignore, c.notify, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { w.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done; _ = w.Close() })
	for _, tg := range targets {
		w.Add(tg)
	}
	return w
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func resolved(t *testing.T, dir string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestWatcher_ReportsEditsInNewSubdirectories(t *testing.T) {
	root := filepath.Join(resolved(t, t.TempDir()), ".ai-rulez")
	mustWrite(t, filepath.Join(root, "config.toml"), "a")
	c := newCollector()
	startWatcher(t, nil, c, Target{Path: root})

	mustWrite(t, filepath.Join(root, "config.toml"), "b")
	c.waitFor(t, filepath.Join(root, "config.toml"))

	sub := filepath.Join(root, "domains", "backend", "rules")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	c.waitFor(t, filepath.Join(root, "domains"))
	// The new directories must be watched: a later edit inside is reported.
	file := filepath.Join(sub, "r.md")
	deadline := time.Now().Add(eventTimeout)
	for !c.seen(file) && time.Now().Before(deadline) {
		mustWrite(t, file, time.Now().String())
		time.Sleep(50 * time.Millisecond)
	}
	if !c.seen(file) {
		t.Fatal("edit inside a new subdirectory was not reported")
	}
}

func TestWatcher_IgnoresEditorTempFilesAndCustomPatterns(t *testing.T) {
	root := filepath.Join(resolved(t, t.TempDir()), ".ai-rulez")
	mustWrite(t, filepath.Join(root, "x"), "")
	c := newCollector()
	startWatcher(t, func(p string) bool { return filepath.Base(p) == ".generated-manifest.json" }, c, Target{Path: root})

	for _, name := range []string{"rule.md.swp", "rule.md~", ".#rule.md", "4913", ".generated-manifest.json"} {
		mustWrite(t, filepath.Join(root, name), "x")
	}
	real := filepath.Join(root, "rule.md")
	mustWrite(t, real, "x")
	c.waitFor(t, real)

	for _, name := range []string{"rule.md.swp", "rule.md~", ".#rule.md", "4913", ".generated-manifest.json"} {
		if c.seen(filepath.Join(root, name)) {
			t.Errorf("%s must be ignored", name)
		}
	}
}

func TestWatcher_ReattachesRecreatedDirectory(t *testing.T) {
	parent := resolved(t, t.TempDir())
	root := filepath.Join(parent, ".ai-rulez")
	mustWrite(t, filepath.Join(root, "config.toml"), "a")
	c := newCollector()
	startWatcher(t, nil, c, Target{Path: root})

	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	c.waitFor(t, root)
	mustWrite(t, filepath.Join(root, "config.toml"), "b")
	c.waitFor(t, root)

	file := filepath.Join(root, "rules", "late.md")
	deadline := time.Now().Add(eventTimeout)
	for !c.seen(file) && time.Now().Before(deadline) {
		mustWrite(t, file, time.Now().String())
		time.Sleep(50 * time.Millisecond)
	}
	if !c.seen(file) {
		t.Fatal("edits after the directory was re-created were not reported")
	}
}

func TestWatcher_FileTargetOnlyReportsThatFile(t *testing.T) {
	dir := resolved(t, t.TempDir())
	cfg := filepath.Join(dir, "ai-rulez.yaml")
	mustWrite(t, cfg, "a")
	c := newCollector()
	startWatcher(t, nil, c, Target{Path: cfg, File: true})

	other := filepath.Join(dir, "CLAUDE.md")
	mustWrite(t, other, "generated")
	mustWrite(t, cfg, "b")
	c.waitFor(t, cfg)

	if c.seen(other) {
		t.Error("an unrelated sibling file must not be reported")
	}
}

func TestWatch_RunsInitiallyThenOnChangeAndStopsOnCancel(t *testing.T) {
	root := filepath.Join(resolved(t, t.TempDir()), ".ai-rulez")
	mustWrite(t, filepath.Join(root, "config.toml"), "a")

	var mu sync.Mutex
	var runs [][]string
	ran := make(chan struct{}, 16)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Watch(ctx, Options{
			Targets:  func() []Target { return []Target{{Path: root}} },
			Debounce: 20 * time.Millisecond,
			Run: func(_ context.Context, tr []string) error {
				mu.Lock()
				runs = append(runs, tr)
				mu.Unlock()
				ran <- struct{}{}
				return nil
			},
		})
	}()

	waitRun := func() {
		select {
		case <-ran:
		case <-time.After(eventTimeout):
			t.Fatal("timed out waiting for a run")
		}
	}
	waitRun() // initial
	mu.Lock()
	if runs[0][0] != "initial" {
		t.Errorf("first trigger = %v, want initial", runs[0])
	}
	mu.Unlock()

	mustWrite(t, filepath.Join(root, "config.toml"), "b")
	waitRun()

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(eventTimeout):
		t.Fatal("Watch did not stop after cancellation")
	}
}

func TestDefaultIgnore(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/p/.ai-rulez/rules/a.md", false},
		{"/p/.ai-rulez/rules/a.md.swp", true},
		{"/p/.ai-rulez/rules/a.md~", true},
		{"/p/.ai-rulez/rules/.#a.md", true},
		{"/p/.ai-rulez/rules/#a.md#", true},
		{"/p/.ai-rulez/.DS_Store", true},
		{"/p/.git/HEAD", true},
		{"/p/.ai-rulez/config.local.toml", false},
	}
	for _, tt := range tests {
		if got := DefaultIgnore(tt.path); got != tt.want {
			t.Errorf("DefaultIgnore(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

// waitForEdit rewrites path until it is reported (a new directory may not be
// watched yet when the first write happens).
func waitForEdit(t *testing.T, c *collector, path string) {
	t.Helper()
	deadline := time.Now().Add(eventTimeout)
	for !c.seen(path) && time.Now().Before(deadline) {
		mustWrite(t, path, time.Now().String())
		time.Sleep(50 * time.Millisecond)
	}
	if !c.seen(path) {
		t.Fatalf("edit of %s was not reported; saw %v", path, c.paths)
	}
}

func TestWatcher_RootBelowAnIgnoredDirectoryName(t *testing.T) {
	// Arrange: the project itself lives under node_modules and a .git directory
	// (a vendored checkout, a worktree); only components below the root count.
	root := filepath.Join(resolved(t, t.TempDir()), "node_modules", ".git", "proj", ".ai-rulez")
	mustWrite(t, filepath.Join(root, "config.toml"), "a")
	c := newCollector()
	startWatcher(t, nil, c, Target{Path: root})

	// Act
	mustWrite(t, filepath.Join(root, "config.toml"), "b")
	mustWrite(t, filepath.Join(root, "node_modules", "pkg", "x.md"), "ignored")
	mustWrite(t, filepath.Join(root, "rules", "r.md"), "kept")

	// Assert
	c.waitFor(t, filepath.Join(root, "config.toml"))
	waitForEdit(t, c, filepath.Join(root, "rules", "r.md"))
	if c.seen(filepath.Join(root, "node_modules", "pkg", "x.md")) {
		t.Error("a node_modules directory below the root must still be ignored")
	}
}

func TestWatcher_FollowsSymlinkedTargetAndSubdirectories(t *testing.T) {
	// Arrange
	base := resolved(t, t.TempDir())
	realRoot := filepath.Join(base, "real-config")
	linkedRoot := filepath.Join(base, "link-config")
	external := filepath.Join(base, "shared-rules")
	mustWrite(t, filepath.Join(realRoot, "config.toml"), "a")
	mustWrite(t, filepath.Join(external, "r.md"), "a")
	testutil.SymlinkOrSkip(t, realRoot, linkedRoot)
	testutil.SymlinkOrSkip(t, external, filepath.Join(realRoot, "rules"))
	// A link back to an ancestor must not make the walk loop.
	testutil.SymlinkOrSkip(t, realRoot, filepath.Join(external, "loop"))
	c := newCollector()
	startWatcher(t, nil, c, Target{Path: linkedRoot})

	// Act + Assert: the root is resolved, and the linked subdirectory is watched.
	mustWrite(t, filepath.Join(realRoot, "config.toml"), "b")
	c.waitFor(t, filepath.Join(realRoot, "config.toml"))
	// Some backends report a change in a linked directory for the directory
	// itself rather than for the file, so any path at or below it counts.
	linked := filepath.Join(realRoot, "rules")
	deadline := time.Now().Add(eventTimeout)
	for !c.seenUnder(linked) && time.Now().Before(deadline) {
		mustWrite(t, filepath.Join(external, "r.md"), time.Now().String())
		time.Sleep(50 * time.Millisecond)
	}
	if !c.seenUnder(linked) {
		t.Fatalf("a change in the symlinked subdirectory was not reported; saw %v", c.paths)
	}
}

func TestWatcher_SyncPrunesRemovedTargets(t *testing.T) {
	// Arrange
	base := resolved(t, t.TempDir())
	kept, dropped := filepath.Join(base, "kept"), filepath.Join(base, "dropped", "src")
	mustWrite(t, filepath.Join(kept, "a.md"), "a")
	mustWrite(t, filepath.Join(dropped, "b.md"), "b")
	c := newCollector()
	w := startWatcher(t, nil, c, Target{Path: kept}, Target{Path: dropped})

	// Act
	w.Sync([]Target{{Path: kept}})
	mustWrite(t, filepath.Join(dropped, "b.md"), "changed")
	mustWrite(t, filepath.Join(kept, "a.md"), "changed")

	// Assert
	c.waitFor(t, filepath.Join(kept, "a.md"))
	time.Sleep(200 * time.Millisecond)
	if c.seen(filepath.Join(dropped, "b.md")) {
		t.Error("a target that was removed must stop reporting")
	}
	if w.isWatched(dropped) || w.isWatched(filepath.Dir(dropped)) {
		t.Error("the directories of a removed target must be released")
	}
}

func TestWatcher_WarnsOncePerRunWhenAWatchCannotBeAdded(t *testing.T) {
	// Arrange
	root := filepath.Join(resolved(t, t.TempDir()), ".ai-rulez")
	for _, name := range []string{"a", "b", "c"} {
		mustWrite(t, filepath.Join(root, name, "x.md"), "x")
	}
	var mu sync.Mutex
	var warnings [][]any
	w, err := NewWatcher(nil, func(string) {}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })
	w.SetWarn(func(msg string, kv ...any) {
		mu.Lock()
		warnings = append(warnings, append([]any{msg}, kv...))
		mu.Unlock()
	})
	w.add = func(string) error { return &os.PathError{Op: "add", Path: "x", Err: syscall.ENOSPC} }

	// Act
	w.Add(Target{Path: root})

	// Assert
	mu.Lock()
	defer mu.Unlock()
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want exactly one for several failures", warnings)
	}
	text := ""
	for _, v := range warnings[0] {
		if s, ok := v.(string); ok {
			text += s + " "
		}
	}
	for _, want := range []string{"ulimit", "inotify"} {
		if !strings.Contains(text, want) {
			t.Errorf("warning %q lacks the %s hint", text, want)
		}
	}
}

func TestWatcher_MissingDirectoryIsNotAWarning(t *testing.T) {
	// Arrange: a target that does not exist yet is normal, not a limit problem.
	w, err := NewWatcher(nil, func(string) {}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })
	warned := false
	w.SetWarn(func(string, ...any) { warned = true })
	w.add = func(string) error { return &os.PathError{Op: "add", Path: "x", Err: os.ErrNotExist} }

	// Act
	w.Add(Target{Path: filepath.Join(resolved(t, t.TempDir()), "later")})

	// Assert
	if warned {
		t.Error("a missing directory must not warn")
	}
}
