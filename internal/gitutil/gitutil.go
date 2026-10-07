// Package gitutil answers the few repository questions generation needs (is a
// path tracked, is it ignored, where is the exclude file) by shelling out to git.
// "Not a repository" and "git is not installed" are normal answers (nothing is
// tracked, no exclude file exists); a git command that fails inside a repository
// is reported as an error so callers can fail closed.
package gitutil

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

const (
	commandTimeout = 30 * time.Second
	// pathChunk bounds how many paths one git invocation receives.
	pathChunk = 200
)

// maxIgnoreFileSize caps how much of one ignore file is mirrored; git itself
// stops reading a .gitignore at 100 MB. A var so tests can lower it.
var maxIgnoreFileSize int64 = 100 << 20

// run executes git in dir through the Git's runner. exitCode distinguishes "no
// match" (1) from failure (>1) for the commands that use it; err is set when git
// could not run or exited non-zero.
func (g Git) run(ctx context.Context, dir string, stdin []byte, args ...string) (out []byte, exitCode int, err error) {
	res := g.runner().Run(ctx, runner.Spec{
		Argv:    append([]string{gitProgram}, gitArgs(dir, args)...),
		Env:     Env(nil),
		Stdin:   stdin,
		Timeout: commandTimeout,
	})
	switch res.Status {
	case runner.StatusOK:
		return res.Stdout, 0, nil
	case runner.StatusExit:
		return res.Stdout, res.ExitCode, oops.With("args", strings.Join(args, " ")).Wrapf(res.Err, "git failed: %s", strings.TrimSpace(string(res.Stderr)))
	default:
		return nil, -1, oops.With("args", strings.Join(args, " ")).Wrapf(res.Err, "run git")
	}
}

// IsRepoContext reports whether dir is inside a git work tree. A missing git binary or
// a directory outside any repository is simply "no".
func (g Git) IsRepoContext(ctx context.Context, dir string) bool {
	out, _, err := g.run(ctx, dir, nil, "rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(string(out)) == "true"
}

// TopLevelContext returns the work tree root containing dir, or "" outside a repository.
func (g Git) TopLevelContext(ctx context.Context, dir string) string {
	out, _, err := g.run(ctx, dir, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return ""
	}
	return filepath.Clean(strings.TrimSpace(string(out)))
}

// literalSpecs turns slash-separated relative paths into literal pathspecs so
// glob characters in a file name are not interpreted.
func literalSpecs(paths []string) []string {
	specs := make([]string, len(paths))
	for i, p := range paths {
		specs[i] = ":(literal)" + p
	}
	return specs
}

// TrackedAmongContext returns which of paths (slash-separated, relative to dir) git
// tracks. Outside a repository nothing is tracked. When git fails inside a
// repository the error is returned: callers should treat every candidate as
// tracked.
func (g Git) TrackedAmongContext(ctx context.Context, dir string, paths []string) (map[string]bool, error) {
	tracked := map[string]bool{}
	if len(paths) == 0 || !g.IsRepoContext(ctx, dir) {
		return tracked, nil
	}
	for start := 0; start < len(paths); start += pathChunk {
		end := min(start+pathChunk, len(paths))
		args := append([]string{"ls-files", "-z", "--"}, literalSpecs(paths[start:end])...)
		out, _, err := g.run(ctx, dir, nil, args...)
		if err != nil {
			return nil, err
		}
		for _, name := range strings.Split(string(out), "\x00") {
			if name != "" {
				tracked[filepath.ToSlash(name)] = true
			}
		}
	}
	return tracked, nil
}

// IgnoredAmongContext returns which of paths (slash-separated, relative to dir) git
// ignores, per .gitignore files, .git/info/exclude and the global excludes file,
// in one call. Tracked files are judged by the patterns too (--no-index). It
// returns (nil, nil) outside a repository so callers can fall back to their own
// matcher.
func (g Git) IgnoredAmongContext(ctx context.Context, dir string, paths []string) (map[string]bool, error) {
	if len(paths) == 0 || !g.IsRepoContext(ctx, dir) {
		return nil, nil
	}
	var stdin bytes.Buffer
	for _, p := range paths {
		stdin.WriteString(p)
		stdin.WriteByte(0)
	}
	out, code, err := g.run(ctx, dir, stdin.Bytes(), "check-ignore", "--no-index", "--stdin", "-z")
	if err != nil && code != 1 { // exit status 1 means "none of them is ignored"
		return nil, err
	}
	ignored := map[string]bool{}
	for _, name := range strings.Split(string(out), "\x00") {
		if name != "" {
			ignored[filepath.ToSlash(name)] = true
		}
	}
	return ignored, nil
}

// IgnoreMatch is the last ignore rule git found for one path.
type IgnoreMatch struct {
	Source  string // file holding the rule; empty when nothing matched
	Line    int
	Pattern string // as written, "!" prefix included; empty when nothing matched
}

// Matched reports whether any rule matched the path.
func (m IgnoreMatch) Matched() bool { return m.Pattern != "" }

// Negated reports whether the last matching rule un-ignores the path.
func (m IgnoreMatch) Negated() bool { return strings.HasPrefix(m.Pattern, "!") }

// Ignored reports whether the last matching rule ignores the path.
func (m IgnoreMatch) Ignored() bool { return m.Matched() && !m.Negated() }

// IgnoreRulesContext returns, for each of paths (slash-separated, relative to dir), the
// last rule git matched across .gitignore files, .git/info/exclude and the global
// excludes file, negations included. Paths nothing matched map to a zero
// IgnoreMatch. It returns (nil, nil) outside a repository.
func (g Git) IgnoreRulesContext(ctx context.Context, dir string, paths []string) (map[string]IgnoreMatch, error) {
	if len(paths) == 0 || !g.IsRepoContext(ctx, dir) {
		return nil, nil
	}
	return g.checkIgnore(ctx, dir, nil, paths)
}

// checkIgnore runs one batched verbose check-ignore in dir.
func (g Git) checkIgnore(ctx context.Context, dir string, gitFlags, paths []string) (map[string]IgnoreMatch, error) {
	var stdin bytes.Buffer
	for _, p := range paths {
		stdin.WriteString(p)
		stdin.WriteByte(0)
	}
	args := append(append([]string{}, gitFlags...), "check-ignore", "--no-index", "-v", "-n", "--stdin", "-z")
	out, code, err := g.run(ctx, dir, stdin.Bytes(), args...)
	if err != nil && code != 1 { // exit status 1 means "none of them is ignored"
		return nil, err
	}
	// -z -v -n emits <source> <line> <pattern> <path>, each NUL-terminated.
	fields := strings.Split(string(out), "\x00")
	rules := make(map[string]IgnoreMatch, len(paths))
	for i := 0; i+3 < len(fields); i += 4 {
		line := 0
		if n, convErr := strconv.Atoi(fields[i+1]); convErr == nil {
			line = n
		}
		rules[filepath.ToSlash(fields[i+3])] = IgnoreMatch{Source: fields[i], Line: line, Pattern: fields[i+2]}
	}
	return rules, nil
}

// IgnoreRulesMirroredContext is IgnoreRules evaluated against a throwaway copy of the
// repository's ignore files, so the caller can leave out content of its own
// without touching the user's files. rewrite receives each ignore file's path
// (slash-separated, relative to the work tree root, or "info/exclude" for the
// repository exclude file) and its content, and returns the content to use. The
// global excludes file is honored as configured. Paths are relative to dir. It
// returns (nil, nil) outside a repository.
func (g Git) IgnoreRulesMirroredContext(ctx context.Context, dir string, paths []string, rewrite func(rel, content string) string) (map[string]IgnoreMatch, error) {
	if len(paths) == 0 || !g.IsRepoContext(ctx, dir) {
		return nil, nil
	}
	top := g.TopLevelContext(ctx, dir)
	prefix := RepoRelative(top, dir)
	if top == "" || prefix == "" {
		return nil, oops.Errorf("cannot place %s inside its repository", dir)
	}
	if prefix == "." {
		prefix = ""
	} else {
		prefix += "/"
	}

	mirror, err := os.MkdirTemp("", "ai-rulez-ignore-")
	if err != nil {
		return nil, oops.Wrapf(err, "create ignore mirror")
	}
	defer os.RemoveAll(mirror) //nolint:errcheck // best-effort cleanup of a temp dir
	if err := g.fillIgnoreMirror(ctx, dir, top, mirror, rewrite); err != nil {
		return nil, err
	}

	var flags []string
	if v, _, cfgErr := g.run(ctx, dir, nil, "config", "--get", "--type=path", "core.excludesFile"); cfgErr == nil {
		if path := strings.TrimSpace(string(v)); path != "" {
			flags = []string{"-c", "core.excludesFile=" + path}
		}
	}
	prefixed := make([]string, len(paths))
	for i, p := range paths {
		prefixed[i] = prefix + p
	}
	rules, err := g.checkIgnore(ctx, mirror, flags, prefixed)
	if err != nil {
		return nil, err
	}
	back := make(map[string]IgnoreMatch, len(rules))
	for p, m := range rules {
		back[strings.TrimPrefix(p, prefix)] = m
	}
	return back, nil
}

// fillIgnoreMirror makes mirror a repository holding copies of top's ignore
// files (and the exclude file) passed through rewrite.
func (g Git) fillIgnoreMirror(ctx context.Context, dir, top, mirror string, rewrite func(rel, content string) string) error {
	if _, _, err := g.run(ctx, mirror, nil, "init", "-q"); err != nil {
		return err
	}

	// Tracked and untracked-but-not-ignored files cover every .gitignore git
	// would read; one inside an ignored directory is never consulted anyway.
	out, _, err := g.run(ctx, top, nil, "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--", ":(glob)**/.gitignore")
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, rel := range strings.Split(string(out), "\x00") {
		rel = filepath.ToSlash(rel)
		if rel == "" || seen[rel] {
			continue
		}
		seen[rel] = true
		if err := copyRewritten(g.Log, filepath.Join(top, filepath.FromSlash(rel)), filepath.Join(mirror, filepath.FromSlash(rel)), rel, rewrite); err != nil {
			return err
		}
	}
	if exclude := g.InfoExcludePathContext(ctx, dir); exclude != "" {
		if err := copyRewritten(g.Log, exclude, filepath.Join(mirror, ".git", "info", "exclude"), "info/exclude", rewrite); err != nil {
			return err
		}
	}

	return nil
}

// copyRewritten copies src to dst (creating parents) with its content passed
// through rewrite. A missing src is skipped, and so is anything that is not a
// regular file (git does not read a symlinked ignore file, and following one to
// a device or a huge file would exhaust memory) or is larger than
// maxIgnoreFileSize.
func copyRewritten(log logger.Logger, src, dst, rel string, rewrite func(rel, content string) string) error {
	info, err := os.Lstat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return oops.With("path", src).Wrapf(err, "stat ignore file")
	}
	if !info.Mode().IsRegular() {
		logger.Or(log).Debug("Skipping an ignore file that is not a regular file", "path", src)
		return nil
	}
	f, err := os.Open(src) //nolint:gosec // ignore file located through git
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return oops.With("path", src).Wrapf(err, "read ignore file")
	}
	defer f.Close() //nolint:errcheck // read-only handle
	data, err := io.ReadAll(io.LimitReader(f, maxIgnoreFileSize+1))
	if err != nil {
		return oops.With("path", src).Wrapf(err, "read ignore file")
	}
	if int64(len(data)) > maxIgnoreFileSize {
		logger.Or(log).Warn("Skipping an ignore file larger than the size limit", "path", src, "limit", maxIgnoreFileSize)
		return nil
	}
	content := string(data)
	if rewrite != nil {
		content = rewrite(rel, content)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return oops.Wrapf(err, "create ignore mirror directory")
	}
	if err := os.WriteFile(dst, []byte(content), 0o600); err != nil {
		return oops.With("path", dst).Wrapf(err, "write ignore mirror")
	}
	return nil
}

// InfoExcludePathContext returns the repository's info/exclude file, resolved through
// git so linked worktrees (whose .git is a file) get the shared one. It returns
// "" outside a repository.
func (g Git) InfoExcludePathContext(ctx context.Context, dir string) string {
	out, _, err := g.run(ctx, dir, nil, "rev-parse", "--git-path", "info/exclude")
	if err != nil {
		return ""
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		return ""
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	return filepath.Clean(path)
}

// RepoRelative converts an absolute path to a slash-separated path relative to
// the given work tree root (from TopLevel), or "" when it is not below it. The
// path need not exist.
func RepoRelative(top, absPath string) string {
	if top == "" {
		return ""
	}
	rel, err := filepath.Rel(Resolve(top), Resolve(absPath))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	return filepath.ToSlash(rel)
}

// Resolve resolves symlinks in the longest existing ancestor of path and
// re-appends the rest, so paths that do not exist yet compare correctly.
func Resolve(path string) string {
	rest := ""
	for p := path; ; p = filepath.Dir(p) {
		if resolved, err := filepath.EvalSymlinks(p); err == nil {
			return filepath.Join(resolved, rest)
		}
		rest = filepath.Join(filepath.Base(p), rest)
		if filepath.Dir(p) == p {
			return path
		}
	}
}

// TrackedFilesContext returns every path in the git index below dir, slash-separated
// and relative to dir, mapped to its git file mode (0o100644, 0o100755,
// 0o120000 for a symlink, ...). ok is false outside a repository, where
// callers fall back to walking the file system. The index is the source, so a
// path listed there counts even when its working-tree file is absent.
func (g Git) TrackedFilesContext(ctx context.Context, dir string) (files map[string]uint32, ok bool, err error) {
	if !g.IsRepoContext(ctx, dir) {
		return nil, false, nil
	}
	out, _, err := g.run(ctx, dir, nil, "ls-files", "-s", "-z")
	if err != nil {
		return nil, true, err
	}
	files = map[string]uint32{}
	for _, entry := range strings.Split(string(out), "\x00") {
		meta, name, found := strings.Cut(entry, "\t")
		if !found {
			continue
		}
		fields := strings.Fields(meta)
		if len(fields) == 0 {
			continue
		}
		mode, perr := strconv.ParseUint(fields[0], 8, 32)
		if perr != nil {
			continue
		}
		files[filepath.ToSlash(name)] = uint32(mode)
	}
	return files, true, nil
}

// IsLinkedWorktreeContext reports whether dir is inside a linked git worktree (one made
// with `git worktree add`) rather than the main checkout. Outside a repository,
// or when git cannot run, it is false.
func (g Git) IsLinkedWorktreeContext(ctx context.Context, dir string) bool {
	gitDir := g.gitPath(ctx, dir, "--git-dir")
	common := g.gitPath(ctx, dir, "--git-common-dir")
	return gitDir != "" && common != "" && gitDir != common
}

// gitPath resolves the path `git rev-parse <flag>` prints, which may be
// relative to dir, to a symlink-free absolute path.
func (g Git) gitPath(ctx context.Context, dir, flag string) string {
	out, _, err := g.run(ctx, dir, nil, "rev-parse", flag)
	if err != nil {
		return ""
	}
	p := strings.TrimSpace(string(out))
	if p == "" {
		return ""
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(dir, p)
	}
	return Resolve(p)
}

// ShowFileContext returns the content of repoRelPath at ref (for example "HEAD"), read
// from the repository containing dir. ok is false when the path does not exist
// at that ref, the ref is unknown, or git cannot run. A ref that is empty or
// starts with "-" is refused without running git: "<ref>:<path>" would be read
// as an option.
func (g Git) ShowFileContext(ctx context.Context, dir, ref, repoRelPath string) (content []byte, ok bool) {
	if strings.TrimSpace(ref) == "" || strings.HasPrefix(ref, "-") {
		return nil, false
	}
	out, _, err := g.run(ctx, dir, nil, "show", ref+":"+filepath.ToSlash(repoRelPath))
	if err != nil {
		return nil, false
	}
	return out, true
}

// ChangedSinceContext lists the files that differ between the merge-base of rev
// and HEAD and the working tree
// (committed, staged and unstaged changes, including deletions) plus untracked
// files that are not ignored, as slash paths relative to the repository root.
// dir may be any directory inside the repository. An unknown rev, a directory
// outside a repository, or a rev that looks like an option is an error.
func (g Git) ChangedSinceContext(ctx context.Context, dir, rev string) ([]string, error) {
	rev = strings.TrimSpace(rev)
	if rev == "" || strings.HasPrefix(rev, "-") {
		return nil, oops.Errorf("invalid git revision %q", rev)
	}
	top := g.TopLevelContext(ctx, dir)
	if top == "" {
		return nil, oops.Errorf("%s is not inside a git repository", dir)
	}
	// Diff against the merge-base so a base branch that moved on since the fork
	// does not count its own changes; without a common ancestor (a shallow
	// clone) the rev itself is used.
	base := rev
	if mb, _, mbErr := g.run(ctx, top, nil, "merge-base", rev, "HEAD"); mbErr == nil && strings.TrimSpace(string(mb)) != "" {
		base = strings.TrimSpace(string(mb))
	}
	diff, _, err := g.run(ctx, top, nil, "diff", "--name-only", "-z", "--no-renames", base, "--")
	if err != nil {
		return nil, oops.Hint("Check that the revision exists (git rev-parse --verify "+rev+")").Wrapf(err, "list files changed since %s", rev)
	}
	others, _, err := g.run(ctx, top, nil, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, oops.Wrapf(err, "list untracked files")
	}
	seen := map[string]bool{}
	var out []string
	for _, chunk := range [][]byte{diff, others} {
		for _, p := range strings.Split(string(chunk), "\x00") {
			if p != "" && !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	return out, nil
}

// StageExecutableContext records the executable bit for a tracked file in the index
// (`git update-index --chmod=+x`), which is what git, and tools that read the
// index mode, see; the working-tree mode alone does not change it. It reports
// whether the index changed. An untracked file, a file already executable in
// the index, and a path outside a repository are not errors: nothing to do.
func (g Git) StageExecutableContext(ctx context.Context, absPath string) (changed bool, err error) {
	dir := filepath.Dir(absPath)
	top := g.TopLevelContext(ctx, dir)
	if top == "" {
		return false, nil
	}
	rel, relErr := filepath.Rel(Resolve(top), Resolve(absPath))
	if relErr != nil || strings.HasPrefix(rel, "..") {
		return false, nil //nolint:nilerr // outside this repository: not ours to stage
	}
	rel = filepath.ToSlash(rel)
	out, _, err := g.run(ctx, top, nil, "ls-files", "-s", "--", ":(literal)"+rel)
	if err != nil {
		return false, oops.Wrapf(err, "read index mode of %s", rel)
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 || fields[0] != "100644" {
		return false, nil
	}
	if _, _, err := g.run(ctx, top, nil, "update-index", "--chmod=+x", "--", rel); err != nil {
		return false, oops.Wrapf(err, "stage executable bit of %s", rel)
	}
	return true, nil
}

// IsRepo is IsRepoContext without a caller's context.
func (g Git) IsRepo(dir string) bool {
	return g.IsRepoContext(context.Background(), dir)
}

// TopLevel is TopLevelContext without a caller's context.
func (g Git) TopLevel(dir string) string {
	return g.TopLevelContext(context.Background(), dir)
}

// TrackedAmong is TrackedAmongContext without a caller's context.
func (g Git) TrackedAmong(dir string, paths []string) (map[string]bool, error) {
	return g.TrackedAmongContext(context.Background(), dir, paths)
}

// IgnoredAmong is IgnoredAmongContext without a caller's context.
func (g Git) IgnoredAmong(dir string, paths []string) (map[string]bool, error) {
	return g.IgnoredAmongContext(context.Background(), dir, paths)
}

// IgnoreRules is IgnoreRulesContext without a caller's context.
func (g Git) IgnoreRules(dir string, paths []string) (map[string]IgnoreMatch, error) {
	return g.IgnoreRulesContext(context.Background(), dir, paths)
}

// IgnoreRulesMirrored is IgnoreRulesMirroredContext without a caller's context.
func (g Git) IgnoreRulesMirrored(dir string, paths []string, rewrite func(rel, content string) string) (map[string]IgnoreMatch, error) {
	return g.IgnoreRulesMirroredContext(context.Background(), dir, paths, rewrite)
}

// InfoExcludePath is InfoExcludePathContext without a caller's context.
func (g Git) InfoExcludePath(dir string) string {
	return g.InfoExcludePathContext(context.Background(), dir)
}

// TrackedFiles is TrackedFilesContext without a caller's context.
func (g Git) TrackedFiles(dir string) (files map[string]uint32, ok bool, err error) {
	return g.TrackedFilesContext(context.Background(), dir)
}

// IsLinkedWorktree is IsLinkedWorktreeContext without a caller's context.
func (g Git) IsLinkedWorktree(dir string) bool {
	return g.IsLinkedWorktreeContext(context.Background(), dir)
}

// ShowFile is ShowFileContext without a caller's context.
func (g Git) ShowFile(dir, ref, repoRelPath string) (content []byte, ok bool) {
	return g.ShowFileContext(context.Background(), dir, ref, repoRelPath)
}

// ChangedSince is ChangedSinceContext without a caller's context.
func (g Git) ChangedSince(dir, rev string) ([]string, error) {
	return g.ChangedSinceContext(context.Background(), dir, rev)
}

// StageExecutable is StageExecutableContext without a caller's context.
func (g Git) StageExecutable(absPath string) (changed bool, err error) {
	return g.StageExecutableContext(context.Background(), absPath)
}
