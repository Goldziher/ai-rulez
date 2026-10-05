package watch

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/samber/oops"
)

// Target is something to watch: a directory (recursively, including
// directories created later) or a single file.
type Target struct {
	Path string
	File bool
}

// Logf receives diagnostics; it may be nil.
type Logf func(msg string, kv ...any)

// Watcher turns fsnotify events on its targets into notify calls. Every target
// also has its parent directory watched (non-recursively) so a target that is
// deleted and re-created is attached again.
//
// Target roots are resolved through symlinks, and symlinked subdirectories are
// followed. DefaultIgnore applies to the part of a path below its target root
// only, so a project that happens to live under a node_modules or .git directory
// is still watched.
type Watcher struct {
	fs        *fsnotify.Watcher
	ignore    func(path string) bool
	messages  chan watcherMessage
	stop      chan struct{}
	closeOnce sync.Once
	closeErr  error
	notify    func(path string)
	logf      Logf
	warn      Logf
	// add attaches one directory to the OS watcher; tests replace it.
	add func(dir string) error

	mu         sync.Mutex
	targets    map[string]Target
	watched    map[string]bool
	warnedAdds bool
	// aliases maps the resolved path of a symlinked directory to the path it was
	// reached through. A symlinked directory is watched by its resolved path (the
	// backends differ in how they treat a link), so its events carry the resolved
	// path and are translated back.
	aliases map[string]string
}

// limitHint explains the usual cause of a watch that cannot be added.
const limitHint = "the operating system's limit on watched directories may be reached: " +
	"raise fs.inotify.max_user_watches (Linux) or the open-file limit with ulimit -n (macOS, BSD), " +
	"or reduce the watched tree"

// NewWatcher creates a Watcher. ignore (may be nil) filters event paths in
// addition to DefaultIgnore; notify is called for every relevant change.
func NewWatcher(ignore func(path string) bool, notify func(path string), logf Logf) (*Watcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, oops.Wrapf(err, "create file watcher")
	}
	if logf == nil {
		logf = func(string, ...any) {}
	}
	watcher := &Watcher{
		fs:       w,
		messages: make(chan watcherMessage),
		stop:     make(chan struct{}),
		ignore:   ignore,
		notify:   notify,
		logf:     logf,
		warn:     logf,
		add:      w.Add,
		targets:  map[string]Target{},
		watched:  map[string]bool{},
		aliases:  map[string]string{},
	}
	go watcher.drainEvents()
	return watcher, nil
}

// SetWarn sets where warnings go (default: the Logf given to NewWatcher). A
// warning is a problem the user can act on, such as a watch limit.
func (w *Watcher) SetWarn(warn Logf) {
	if warn != nil {
		w.warn = warn
	}
}

// Close stops the underlying watcher.
func (w *Watcher) Close() error {
	w.closeOnce.Do(func() {
		// Keep draining until the backend has finished: Windows Close also
		// waits for the event reader to acknowledge its request.
		w.closeErr = w.fs.Close()
		close(w.stop)
	})
	if w.closeErr != nil {
		return oops.Wrapf(w.closeErr, "close file watcher")
	}
	return nil
}

// resolveTarget makes the target path absolute and resolves symlinks, through
// the parent directory when the target does not exist yet.
func resolveTarget(t Target) (Target, error) {
	abs, err := filepath.Abs(t.Path)
	if err != nil {
		return t, err //nolint:wrapcheck // the caller logs it with the path
	}
	if resolved, evalErr := filepath.EvalSymlinks(abs); evalErr == nil {
		abs = resolved
	} else if parent, parentErr := filepath.EvalSymlinks(filepath.Dir(abs)); parentErr == nil {
		abs = filepath.Join(parent, filepath.Base(abs))
	}
	t.Path = abs
	return t, nil
}

// Add starts watching a target. It is idempotent and may be called again after
// the target appears; a target that does not exist yet is attached when its
// parent reports it.
func (w *Watcher) Add(t Target) {
	t, err := resolveTarget(t)
	if err != nil {
		w.logf("watch: cannot resolve path", "path", t.Path, "error", err)
		return
	}
	abs := t.Path
	w.mu.Lock()
	w.targets[abs] = t
	w.mu.Unlock()

	w.addDir(filepath.Dir(abs))
	if t.File {
		return
	}
	if info, err := os.Stat(abs); err == nil && info.IsDir() {
		w.addTree(abs)
	}
}

// Sync makes targets the complete set: new ones are added and ones no longer
// listed (an include that was removed from the configuration) stop being
// watched and release their directories.
func (w *Watcher) Sync(targets []Target) {
	keep := make(map[string]bool, len(targets))
	for _, t := range targets {
		if resolved, err := resolveTarget(t); err == nil {
			keep[resolved.Path] = true
		}
	}
	w.mu.Lock()
	for path := range w.targets {
		if !keep[path] {
			delete(w.targets, path)
		}
	}
	for dir := range w.watched {
		if !w.neededLocked(dir) {
			w.unwatchLocked(dir)
		}
	}
	w.mu.Unlock()
	for _, t := range targets {
		w.Add(t)
	}
}

// neededLocked reports whether dir is still watched for a target: it is, or lies
// below, a directory target, or it is the parent of any target.
func (w *Watcher) neededLocked(dir string) bool {
	for p, t := range w.targets {
		if dir == filepath.Dir(p) {
			return true
		}
		if !t.File && (dir == p || strings.HasPrefix(dir, p+string(filepath.Separator))) {
			return true
		}
	}
	return false
}

func (w *Watcher) isWatched(dir string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.watched[dir]
}

// addDir watches one directory when it exists and is not watched yet.
func (w *Watcher) addDir(dir string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.watched[dir] {
		return
	}
	resolved := dir
	if evaluated, err := filepath.EvalSymlinks(dir); err == nil {
		resolved = evaluated
	}
	if err := w.add(resolved); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			w.logf("watch: directory does not exist yet", "path", dir)
			return
		}
		w.logf("watch: cannot watch directory", "path", dir, "error", err)
		if !w.warnedAdds {
			w.warnedAdds = true
			w.warn("watch: cannot watch some directories, so changes in them will not trigger a run",
				"path", dir, "error", err, "hint", limitHint)
		}
		return
	}
	w.watched[dir] = true
	if resolved != dir {
		w.aliases[resolved] = dir
	}
}

// unwatchLocked forgets a watched directory and releases its OS watch, unless
// another watched path (a second link to it) still needs the same resolved path.
func (w *Watcher) unwatchLocked(dir string) {
	resolved := dir
	for r, via := range w.aliases {
		if via == dir {
			resolved = r
			delete(w.aliases, r)
		}
	}
	delete(w.watched, dir)
	for other := range w.watched {
		if otherResolved, err := filepath.EvalSymlinks(other); err == nil && otherResolved == resolved {
			w.aliases[resolved] = other
			return
		}
	}
	if err := w.fs.Remove(resolved); err != nil && !errors.Is(err, fsnotify.ErrNonExistentWatch) {
		w.logf("watch: cannot remove directory watch", "path", resolved, "error", err)
	}
}

// translate maps an event path under the resolved path of a symlinked directory to
// the path the directory was reached through, so it falls under its target.
func (w *Watcher) translate(path string) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	best := ""
	for resolved := range w.aliases {
		if (path == resolved || strings.HasPrefix(path, resolved+string(filepath.Separator))) && len(resolved) > len(best) {
			best = resolved
		}
	}
	if best == "" {
		return path
	}
	return filepath.Join(w.aliases[best], strings.TrimPrefix(path, best))
}

// addTree watches dir and every directory below it, following symlinked
// directories. A link that leads back to a directory already on the path being
// walked is skipped, so cycles terminate.
func (w *Watcher) addTree(dir string) {
	w.walk(dir, dir, map[string]bool{})
}

func (w *Watcher) walk(root, dir string, ancestors map[string]bool) {
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil || ancestors[resolved] {
		return
	}
	if dir != root && w.isIgnored(dir) {
		return
	}
	ancestors[resolved] = true
	defer delete(ancestors, resolved)
	w.addDir(dir)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return // a vanished or unreadable directory is simply not watched further
	}
	for _, entry := range entries {
		child := filepath.Join(dir, entry.Name())
		isDir := entry.IsDir()
		if !isDir && entry.Type()&fs.ModeSymlink != 0 {
			if info, statErr := os.Stat(child); statErr == nil && info.IsDir() {
				isDir = true
			}
		}
		if isDir {
			w.walk(root, child, ancestors)
		}
	}
}

// isIgnored applies the custom filter to the full path and DefaultIgnore to the
// part below the target root that contains it.
func (w *Watcher) isIgnored(path string) bool {
	if w.ignore != nil && w.ignore(path) {
		return true
	}
	return DefaultIgnore(w.belowRoot(path))
}

// belowRoot is path relative to the innermost target that contains it, or just
// its base name when none does (the parent directory of a target).
func (w *Watcher) belowRoot(path string) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	best := ""
	for p, t := range w.targets {
		if path == p {
			return filepath.Base(path)
		}
		if !t.File && strings.HasPrefix(path, p+string(filepath.Separator)) && len(p) > len(best) {
			best = p
		}
	}
	if best == "" {
		return filepath.Base(path)
	}
	rel, err := filepath.Rel(best, path)
	if err != nil {
		return filepath.Base(path)
	}
	return rel
}

// relevant reports whether path is a target or lies inside a directory target.
func (w *Watcher) relevant(path string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	for p, t := range w.targets {
		if path == p {
			return true
		}
		if !t.File && strings.HasPrefix(path, p+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// forget drops the watch bookkeeping for a removed path and everything below it.
func (w *Watcher) forget(path string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for p := range w.watched {
		if p == path || strings.HasPrefix(p, path+string(filepath.Separator)) {
			w.unwatchLocked(p)
		}
	}
}

// Run processes events until ctx is canceled.
func (w *Watcher) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case message, ok := <-w.messages:
			if !ok {
				return
			}
			if message.err != nil {
				w.logf("watch: watcher error", "error", message.err)
			} else {
				w.handle(message.event)
			}
		}
	}
}

func (w *Watcher) handle(ev fsnotify.Event) {
	path := filepath.Clean(ev.Name)
	if ev.Op == fsnotify.Chmod {
		return
	}
	if !w.relevant(path) {
		if path = w.translate(path); !w.relevant(path) {
			return
		}
	}
	if w.isIgnored(path) {
		return
	}
	if ev.Has(fsnotify.Remove) || ev.Has(fsnotify.Rename) {
		w.forget(path)
	}
	if ev.Has(fsnotify.Create) {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			// Files created before the new directory was watched produce no
			// event, so the whole tree is scanned and the change reported.
			w.addTree(path)
		}
	}
	w.notify(path)
}
