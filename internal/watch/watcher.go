package watch

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
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
	fs     *fsnotify.Watcher
	ignore func(path string) bool
	notify func(path string)
	logf   Logf
	warn   Logf
	// add attaches one directory to the OS watcher; tests replace it.
	add func(dir string) error

	mu         sync.Mutex
	targets    map[string]Target
	watched    map[string]bool
	warnedAdds bool
	// budget caps the OS descriptors the watcher may hold (0: no cap); costs
	// holds what each watched directory is charged and used their sum.
	budget       int
	priority     func(dir string) bool
	used         int
	costs        map[string]int
	warnedBudget bool
	// aliases maps the real path of a symlinked directory to the path it was
	// reached through. A symlinked directory is watched by its real path (the
	// backends differ in how they treat a link), so its events carry the real
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
	return &Watcher{
		fs:      w,
		ignore:  ignore,
		notify:  notify,
		logf:    logf,
		warn:    logf,
		add:     w.Add,
		targets: map[string]Target{},
		watched: map[string]bool{},
		costs:   map[string]int{},
		aliases: map[string]string{},
	}, nil
}

// kqueueBackend reports whether fsnotify watches through kqueue, which needs
// one open descriptor for the directory and one for every file in it; inotify
// and the other backends charge one watch per directory.
func kqueueBackend() bool {
	switch runtime.GOOS {
	case "darwin", "freebsd", "netbsd", "openbsd", "dragonfly":
		return true
	}
	return false
}

// dirCost is what watching dir takes from the budget.
func dirCost(dir string) int {
	if !kqueueBackend() {
		return 1
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 1
	}
	return 1 + len(entries)
}

// SetPriority names the directories that are watched first when a budget runs
// out, before the rest in breadth-first order (nil: none). The index watch
// passes the directories that hold skills and instruction files.
func (w *Watcher) SetPriority(priority func(dir string) bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.priority = priority
}

// SetBudget caps the descriptors the watcher may hold, so a large tree cannot
// exhaust the system's file table (kqueue holds one per file). Directories
// beyond the cap are left unwatched, shallowest first, and one warning says so.
// 0 removes the cap.
func (w *Watcher) SetBudget(n int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.budget = n
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
	if err := w.fs.Close(); err != nil {
		return oops.Wrapf(err, "close file watcher")
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
	if realPath, evalErr := filepath.EvalSymlinks(abs); evalErr == nil {
		abs = realPath
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
	realPath := dir
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		realPath = resolved
	}
	cost := 1
	if w.budget > 0 {
		cost = dirCost(realPath)
		if w.used+cost > w.budget {
			if !w.warnedBudget {
				w.warnedBudget = true
				w.warn("watch: descriptor budget reached, so directories beyond it are not watched and their changes are seen only by the periodic poll",
					"path", dir, "budget", w.budget)
			}
			return
		}
	}
	if err := w.add(realPath); err != nil {
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
	w.costs[dir] = cost
	w.used += cost
	if realPath != dir {
		w.aliases[realPath] = dir
	}
}

// unwatchLocked forgets a watched directory and releases its OS watch, unless
// another watched path (a second link to it) still needs the same real path.
func (w *Watcher) unwatchLocked(dir string) {
	realPath := dir
	for r, via := range w.aliases {
		if via == dir {
			realPath = r
			delete(w.aliases, r)
		}
	}
	delete(w.watched, dir)
	w.used -= w.costs[dir]
	delete(w.costs, dir)
	for other := range w.watched {
		if resolved, err := filepath.EvalSymlinks(other); err == nil && resolved == realPath {
			w.aliases[realPath] = other
			return
		}
	}
	if err := w.fs.Remove(realPath); err != nil {
		return // the directory may already be gone
	}
}

// translate maps an event path under the real path of a symlinked directory to
// the path the directory was reached through, so it falls under its target.
func (w *Watcher) translate(path string) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	best := ""
	for real := range w.aliases {
		if (path == real || strings.HasPrefix(path, real+string(filepath.Separator))) && len(real) > len(best) {
			best = real
		}
	}
	if best == "" {
		return path
	}
	return filepath.Join(w.aliases[best], strings.TrimPrefix(path, best))
}

// addTree watches dir and every directory below it, following symlinked
// directories. The directories are found breadth first and watched priority
// directories first, so a descriptor budget runs out in the deepest ordinary
// ones. A link that leads back to a directory already on the path being walked
// is skipped, so cycles terminate.
func (w *Watcher) addTree(dir string) {
	w.mu.Lock()
	priority := w.priority
	w.mu.Unlock()
	for _, d := range prioritized(w.findTree(dir), priority) {
		w.addDir(d)
	}
}

// findTree lists dir and the directories below it that are not ignored,
// breadth first.
func (w *Watcher) findTree(dir string) []string {
	type node struct {
		dir       string
		ancestors []string
	}
	var found []string
	queue := []node{{dir: dir}}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		realPath, err := filepath.EvalSymlinks(n.dir)
		if err != nil || slices.Contains(n.ancestors, realPath) {
			continue
		}
		if n.dir != dir && w.isIgnored(n.dir) {
			continue
		}
		found = append(found, n.dir)

		entries, err := os.ReadDir(n.dir)
		if err != nil {
			continue // a vanished or unreadable directory is simply not watched further
		}
		ancestors := append(slices.Clone(n.ancestors), realPath)
		for _, entry := range entries {
			if child := filepath.Join(n.dir, entry.Name()); isDirOrLinkToDir(child, entry) {
				queue = append(queue, node{dir: child, ancestors: ancestors})
			}
		}
	}
	return found
}

// isDirOrLinkToDir reports whether entry, found at path, is a directory or a
// symlink to one.
func isDirOrLinkToDir(path string, entry fs.DirEntry) bool {
	if entry.IsDir() {
		return true
	}
	if entry.Type()&fs.ModeSymlink == 0 {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// prioritized puts the directories priority names first and keeps the order
// otherwise (nil priority: dirs unchanged).
func prioritized(dirs []string, priority func(dir string) bool) []string {
	if priority == nil {
		return dirs
	}
	first := make([]string, 0, len(dirs))
	var rest []string
	for _, d := range dirs {
		if priority(d) {
			first = append(first, d)
		} else {
			rest = append(rest, d)
		}
	}
	return append(first, rest...)
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
		case ev, ok := <-w.fs.Events:
			if !ok {
				return
			}
			w.handle(ev)
		case err, ok := <-w.fs.Errors:
			if !ok {
				return
			}
			w.logf("watch: watcher error", "error", err)
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
