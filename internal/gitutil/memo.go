package gitutil

import (
	"context"
	"errors"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

const (
	lsFilesCmd = "ls-files"
	cachedFlag = "--cached"
)

// Memo remembers the answers to the structural questions asked of a repository
// (is dir in a work tree, where is its top level, where is info/exclude, which
// git directories): the same few questions are asked of the same directory many
// times in one operation. It is opt-in and scoped by its context: nothing is
// cached unless the caller put a Memo there with WithMemo, and the answers live
// exactly as long as that context, so a command that creates or removes a
// repository starts over with a new one. Questions about history (HEAD, refs,
// file lists) are never memoised; the one exception is the tracked-file listing
// WithSharedIndex opts into.
type Memo struct {
	mu sync.Mutex
	m  map[memoKey]memoVal

	// shared is set by WithSharedIndex: the index listings below are kept.
	shared   bool
	listings map[memoKey]*indexListing
}

// indexListing is one repository's tracked files, read once. mu is held while
// it is read, so concurrent askers wait for the one listing instead of taking
// their own; a failed read is not kept, the next asker tries again.
type indexListing struct {
	mu    sync.Mutex
	files map[string]bool
	// gitlinks holds the submodule entries of the index: a path below one is not
	// the listing's to answer (see trackedFromSharedIndex).
	gitlinks map[string]bool
	names    []string // the untracked files (see othersContext)
	raw      []byte   // the output of a listing kept as it came (see ignoreFileListing)
	done     bool
}

// memoKey names a question. The runner is part of it: two Git values over
// different runners (a fake in a test, a different host) are not asking the same
// repository, so they do not share answers.
type memoKey struct {
	dir, args string
	runner    runner.Runner
}

type memoVal struct {
	out []byte
	err error
}

type memoCtxKey struct{}

// WithMemo returns ctx carrying a fresh Memo, or ctx itself when it has one.
func WithMemo(ctx context.Context) context.Context {
	if memoFrom(ctx) != nil {
		return ctx
	}
	return context.WithValue(ctx, memoCtxKey{}, &Memo{m: make(map[memoKey]memoVal)})
}

// WithSharedIndex is WithMemo that also keeps each repository's tracked-file
// listing, so every question of the form "does git track this file" asked of
// that repository during the operation (TrackedAmongContext) is answered from
// one `git ls-files` instead of one index read each. Use it only for an
// operation that does not stage or unstage files, such as a read-only check
// over several roots of one repository: the listing is not refreshed.
func WithSharedIndex(ctx context.Context) context.Context {
	if m := memoFrom(ctx); m != nil && m.shared {
		return ctx
	}
	return context.WithValue(ctx, memoCtxKey{}, &Memo{
		m: make(map[memoKey]memoVal), shared: true, listings: make(map[memoKey]*indexListing),
	})
}

// listing returns the entry for the repository top, creating it when new.
func (m *Memo) listing(key memoKey) *indexListing {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, ok := m.listings[key]
	if !ok {
		l = &indexListing{}
		m.listings[key] = l
	}
	return l
}

func memoFrom(ctx context.Context) *Memo {
	m, ok := ctx.Value(memoCtxKey{}).(*Memo)
	if !ok {
		return nil
	}
	return m
}

// memoRunner returns the runner as a map key. ok is false for a runner without
// an identity: a func adapter, or a value holding something that cannot be
// compared (using it as a key would panic, or make unrelated runners share an
// answer). Such a runner's questions are never memoised.
func (g Git) memoRunner() (r runner.Runner, ok bool) {
	r = g.runner()
	if !reflect.ValueOf(r).Comparable() {
		return nil, false
	}
	return r, true
}

// revParse runs `git rev-parse args...` in dir, through the context's Memo when
// it has one. Failures are remembered too: "not a repository" is an answer. A
// run cut short by the context's own cancellation or deadline is not: it says
// nothing about the repository.
func (g Git) revParse(ctx context.Context, dir string, args ...string) ([]byte, error) {
	memo := memoFrom(ctx)
	run := func() ([]byte, error) {
		out, _, err := g.run(ctx, dir, nil, append([]string{"rev-parse"}, args...)...)
		return out, err
	}
	if memo == nil {
		return run()
	}
	r, ok := g.memoRunner()
	if !ok {
		return run()
	}
	key := memoKey{dir: dir, args: strings.Join(args, "\x00"), runner: r}
	memo.mu.Lock()
	v, ok := memo.m[key]
	memo.mu.Unlock()
	if ok {
		return v.out, v.err
	}
	out, err := run()
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return out, err
	}
	memo.mu.Lock()
	memo.m[key] = memoVal{out, err}
	memo.mu.Unlock()
	return out, err
}

// trackedFromSharedIndex answers, from the context's shared index listing, which
// of paths git tracks, records the tracked ones in tracked and returns the paths
// it could not answer: all of them without a shared index, and any path that
// leaves dir (".." or absolute), which the listing of dir's repository cannot
// speak for.
func (g Git) trackedFromSharedIndex(ctx context.Context, dir string, paths []string, tracked map[string]bool) []string {
	memo := memoFrom(ctx)
	if memo == nil || !memo.shared {
		return paths
	}
	r, ok := g.memoRunner()
	if !ok {
		return paths
	}
	top := g.TopLevelContext(ctx, dir)
	prefixOut, err := g.revParse(ctx, dir, "--show-prefix")
	if top == "" || err != nil {
		return paths
	}
	l := memo.listing(memoKey{dir: top, args: lsFilesCmd, runner: r})
	if !l.read(func() bool { return g.loadIndexListing(ctx, top, l) }) {
		return paths
	}
	prefix := strings.TrimSpace(string(prefixOut))
	var rest []string
	for _, p := range paths {
		if !insideDir(p) || belowGitlink(l.gitlinks, prefix+p) {
			rest = append(rest, p)
			continue
		}
		if l.files[prefix+p] {
			tracked[p] = true
		}
	}
	return rest
}

// gitlinkMode is the index mode of a submodule entry.
const gitlinkMode = "160000"

// loadIndexListing reads the tracked files of the repository at top into l. It
// reports false when git failed, so the caller falls back to the pathspec call.
func (g Git) loadIndexListing(ctx context.Context, top string, l *indexListing) bool {
	out, _, err := g.run(ctx, top, nil, lsFilesCmd, "-s", "-z")
	if err != nil {
		return false
	}
	l.files, l.gitlinks = make(map[string]bool), make(map[string]bool)
	for _, entry := range strings.Split(string(out), "\x00") {
		meta, name, ok := strings.Cut(entry, "\t") // "<mode> <object> <stage>\t<path>"
		if !ok || name == "" {
			continue
		}
		name = filepath.ToSlash(name)
		l.files[name] = true
		if strings.HasPrefix(meta, gitlinkMode+" ") {
			l.gitlinks[name] = true
		}
	}
	return true
}

// belowGitlink reports whether a proper ancestor directory of the slash path p
// is a submodule entry of the index.
func belowGitlink(gitlinks map[string]bool, p string) bool {
	for dir := path.Dir(p); dir != "." && dir != "/"; dir = path.Dir(dir) {
		if gitlinks[dir] {
			return true
		}
	}
	return false
}

// ignoreFileListing lists the .gitignore files below top that git would read
// (tracked, or untracked and not ignored). With a shared index in ctx the roots
// of one repository take the listing once; the answer is the raw -z output.
func (g Git) ignoreFileListing(ctx context.Context, top string) ([]byte, error) {
	args := []string{lsFilesCmd, "-z", cachedFlag, "--others", "--exclude-standard", "--", ":(glob)**/.gitignore"}
	memo := memoFrom(ctx)
	if memo == nil || !memo.shared {
		out, _, err := g.run(ctx, top, nil, args...)
		return out, err
	}
	r, ok := g.memoRunner()
	if !ok {
		out, _, err := g.run(ctx, top, nil, args...)
		return out, err
	}
	l := memo.listing(memoKey{dir: top, args: "ignore-files", runner: r})
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.done {
		out, _, err := g.run(ctx, top, nil, args...)
		if err != nil {
			return nil, err
		}
		l.raw, l.done = out, true
	}
	return l.raw, nil
}

// insideDir reports whether the slash path p names something below the
// directory it is relative to, spelled the way git prints it.
func insideDir(p string) bool {
	return p != "" && path.Clean(p) == p && p != "." && p != ".." && !strings.HasPrefix(p, "../") && !path.IsAbs(p)
}

// read takes the listing with load the first time, under the listing's lock; it
// reports false when load fails (nothing is kept, the next asker tries again).
func (l *indexListing) read(load func() bool) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.done {
		if !load() {
			return false
		}
		l.done = true
	}
	return true
}
