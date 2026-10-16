package gitutil

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/samber/oops"
)

const (
	// defaultBigIndexBytes is the index size from which the untracked listing is
	// done in process. Below it one `git ls-files --others` is as fast and cheaper.
	defaultBigIndexBytes = 2 << 20
	// specChunk bounds the pathspecs and paths handed to one git process.
	specChunk = 1000
)

// othersContext lists the untracked files below dir that git does not ignore,
// as `git ls-files --others --exclude-standard -z` prints them (slash
// separated, relative to dir, sorted).
func (g Git) othersContext(ctx context.Context, dir string) ([]string, error) {
	return g.sharedOthers(ctx, dir, func() ([]string, error) {
		if !g.bigRepository(dir) {
			return g.othersSingle(ctx, dir)
		}
		out, _, err := g.run(ctx, dir, nil, "ls-files", "--cached", "-z")
		if err != nil {
			return nil, oops.Wrapf(err, "list tracked files")
		}
		return g.othersCompute(ctx, dir, splitNUL(out))
	})
}

// sharedOthers runs list, or answers from the listing a shared index (see
// WithSharedIndex) already holds for dir: one operation asks for the untracked
// files of one directory several times (the files to lint, the files changed),
// and the work tree does not change under it. The slice is shared; callers
// only read it.
func (g Git) sharedOthers(ctx context.Context, dir string, list func() ([]string, error)) ([]string, error) {
	memo := memoFrom(ctx)
	if memo == nil || !memo.shared {
		return list()
	}
	r, ok := g.memoRunner()
	if !ok {
		return list()
	}
	l := memo.listing(memoKey{dir: dir, args: "others", runner: r})
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.done {
		files, err := list()
		if err != nil {
			return nil, err
		}
		l.names, l.done = files, true
	}
	return l.names, nil
}

// othersWith is othersContext for a caller that already holds the tracked
// files below dir.
func (g Git) othersWith(ctx context.Context, dir string, tracked []string) ([]string, error) {
	return g.sharedOthers(ctx, dir, func() ([]string, error) { return g.othersCompute(ctx, dir, tracked) })
}

func (g Git) othersCompute(ctx context.Context, dir string, tracked []string) ([]string, error) {
	if !g.bigRepository(dir) {
		return g.othersSingle(ctx, dir)
	}
	if files, ok := g.othersWalk(ctx, dir, tracked); ok {
		return files, nil
	}
	return g.othersSingle(ctx, dir)
}

func (g Git) othersSingle(ctx context.Context, dir string) ([]string, error) {
	out, _, err := g.run(ctx, dir, nil, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, oops.Wrapf(err, "list untracked files")
	}
	files := splitNUL(out)
	sort.Strings(files)
	return files, nil
}

func splitNUL(out []byte) []string {
	var names []string
	for _, p := range strings.Split(string(out), "\x00") {
		if p != "" {
			names = append(names, p)
		}
	}
	return names
}

// bigRepository reports whether the repository containing dir has an index
// large enough that git's single-threaded walk of the work tree is slow.
func (g Git) bigRepository(dir string) bool {
	limit := int64(defaultBigIndexBytes)
	if g.bigIndexBytes != 0 {
		limit = g.bigIndexBytes
	}
	for d := dir; ; {
		if info, err := os.Lstat(filepath.Join(d, ".git")); err == nil {
			gitDir := filepath.Join(d, ".git")
			if !info.IsDir() { // a linked work tree or submodule: ".git" is a file naming the real one
				data, err := os.ReadFile(gitDir) //nolint:gosec // the repository's own pointer file
				if err != nil {
					return false
				}
				target, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir:")
				if !ok {
					return false
				}
				gitDir = strings.TrimSpace(target)
				if !filepath.IsAbs(gitDir) {
					gitDir = filepath.Join(d, gitDir)
				}
			}
			idx, err := os.Stat(filepath.Join(gitDir, "index"))
			return err == nil && idx.Size() >= limit
		}
		parent := filepath.Dir(d)
		if parent == d {
			return false
		}
		d = parent
	}
}

// othersWalk finds the untracked, not ignored files below dir without asking
// git to walk the whole work tree, which on a large repository costs seconds
// of single-threaded directory reads. It walks the directories that hold
// tracked files in parallel and collects what the index does not know (the
// candidates), then lets git decide: one `check-ignore` drops the ignored
// candidates and `ls-files --others` lists what is left, so the ignore rules,
// nested repositories and case folding are git's. ok is false when git could
// not answer; the caller then takes the single call.
func (g Git) othersWalk(ctx context.Context, dir string, tracked []string) ([]string, bool) {
	known := make(map[string]bool, len(tracked))
	trackedDirs := map[string]bool{"": true}
	for _, p := range tracked {
		known[p] = true
		for d := parentDir(p); d != "" && !trackedDirs[d]; d = parentDir(d) {
			trackedDirs[d] = true
		}
	}
	cands := walkCandidates(dir, known, trackedDirs)
	if len(cands) == 0 {
		return nil, true
	}
	live, ok := g.dropIgnored(ctx, dir, cands)
	if !ok {
		return nil, false
	}
	var files []string
	for start := 0; start < len(live); start += specChunk {
		specs := make([]string, 0, specChunk)
		for _, p := range live[start:min(start+specChunk, len(live))] {
			specs = append(specs, ":(literal)"+p)
		}
		out, _, err := g.run(ctx, dir, nil, append([]string{"ls-files", "--others", "--exclude-standard", "-z", "--"}, specs...)...)
		if err != nil {
			return nil, false
		}
		files = append(files, splitNUL(out)...)
	}
	sort.Strings(files)
	return files, true
}

// parentDir is the slash-separated parent directory of p ("" at the top).
func parentDir(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[:i]
	}
	return ""
}

// dropIgnored returns the candidates git does not ignore, in order.
func (g Git) dropIgnored(ctx context.Context, dir string, cands []string) ([]string, bool) {
	ignored := make(map[string]bool)
	for start := 0; start < len(cands); start += 20 * specChunk {
		chunk := cands[start:min(start+20*specChunk, len(cands))]
		var in bytes.Buffer
		for _, p := range chunk {
			in.WriteString(p)
			in.WriteByte(0)
		}
		out, code, err := g.run(ctx, dir, in.Bytes(), "check-ignore", "--stdin", "-z")
		if err != nil && code != 1 { // exit 1: none of the paths is ignored
			return nil, false
		}
		for _, p := range splitNUL(out) {
			ignored[p] = true
		}
	}
	live := make([]string, 0, len(cands))
	for _, p := range cands {
		if !ignored[p] {
			live = append(live, p)
		}
	}
	return live, true
}

// walkCandidates walks, with one goroutine per CPU, the directories below dir
// that hold tracked files, and returns the slash paths of the entries the index
// does not list: untracked files and links, and directories without a tracked
// file (the whole directory is one candidate). A tracked entry that is a
// directory on disk is delegated to git: it may be a submodule or a directory
// replacing a tracked file. The .git entries are skipped.
func walkCandidates(dir string, known, trackedDirs map[string]bool) []string {
	var (
		mu    sync.Mutex
		cond  = sync.NewCond(&mu)
		queue = []string{""}
		busy  int
		cands []string
	)
	work := func() {
		for {
			mu.Lock()
			for len(queue) == 0 && busy > 0 {
				cond.Wait()
			}
			if len(queue) == 0 {
				mu.Unlock()
				cond.Broadcast()
				return
			}
			rel := queue[len(queue)-1]
			queue = queue[:len(queue)-1]
			busy++
			mu.Unlock()

			var subdirs, found []string
			entries, err := os.ReadDir(filepath.Join(dir, filepath.FromSlash(rel)))
			if err != nil && rel != "" {
				found = append(found, rel) // unreadable: let git say
			}
			for _, e := range entries {
				name := e.Name()
				if name == ".git" {
					continue
				}
				child := name
				if rel != "" {
					child = rel + "/" + name
				}
				switch {
				case known[child] && !e.IsDir():
				case e.IsDir() && trackedDirs[child]:
					subdirs = append(subdirs, child)
				default:
					found = append(found, child)
				}
			}
			mu.Lock()
			queue = append(queue, subdirs...)
			cands = append(cands, found...)
			busy--
			mu.Unlock()
			cond.Broadcast()
		}
	}
	var wg sync.WaitGroup
	for range runtime.GOMAXPROCS(0) {
		wg.Add(1)
		go func() { defer wg.Done(); work() }()
	}
	wg.Wait()
	sort.Strings(cands)
	return cands
}
