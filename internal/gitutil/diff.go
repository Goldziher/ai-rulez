package gitutil

import (
	"bufio"
	"bytes"
	"sort"
	"strconv"
	"strings"

	"github.com/samber/oops"
)

// LineRange is an inclusive range of 1-based line numbers.
type LineRange struct{ Start, End int }

// Change is one file that differs from a base: its path relative to the
// directory the query ran in (slash separated), the path it had before a
// rename, and the lines it gained.
type Change struct {
	Path    string
	OldPath string
	// Status is 'A' (added or untracked), 'M' (modified, including a mode
	// change), 'D' (deleted) or 'R' (renamed, possibly with edits).
	Status byte
	// AllAdded means every line of the file is new (an added or untracked file);
	// Added is empty then.
	AllAdded bool
	// Added lists the new-side line ranges that were added, ascending.
	Added []LineRange
}

// ListFiles returns the files below dir that git knows about: tracked files
// plus untracked files that are not ignored, slash separated, relative to dir,
// sorted. ok is false outside a repository, where callers walk the file system.
// A tracked file deleted from the working tree is still listed; callers check
// that it exists before reading it.
func (g Git) ListFiles(dir string) (files []string, ok bool, err error) {
	if !g.IsRepo(dir) {
		return nil, false, nil
	}
	out, _, err := g.run(dir, nil, "ls-files", "--cached", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, true, oops.Wrapf(err, "list repository files")
	}
	seen := map[string]bool{}
	for _, p := range strings.Split(string(out), "\x00") {
		if p != "" && !seen[p] {
			seen[p] = true
			files = append(files, p)
		}
	}
	sort.Strings(files)
	return files, true, nil
}

// MergeBase returns the commit where HEAD diverged from rev, the base of a
// `rev...HEAD` comparison. An unknown rev, a repository without a HEAD, and a
// history too shallow to contain the common ancestor are all errors, so a
// caller never compares against nothing and passes vacuously.
func (g Git) MergeBase(dir, rev string) (string, error) {
	rev = strings.TrimSpace(rev)
	if rev == "" || strings.HasPrefix(rev, "-") {
		return "", oops.Errorf("invalid git revision %q", rev)
	}
	if !g.IsRepo(dir) {
		return "", oops.Errorf("%s is not inside a git repository", dir)
	}
	hint := "Fetch the base ref (for example `git fetch origin <branch>` or a full-depth checkout) and check `git rev-parse --verify " + rev + "`."
	if _, _, err := g.run(dir, nil, "rev-parse", "--verify", "--quiet", "--end-of-options", rev+"^{commit}"); err != nil {
		return "", oops.Hint(hint).Errorf("base revision %q does not exist in this repository", rev)
	}
	out, _, err := g.run(dir, nil, "merge-base", "--end-of-options", rev, "HEAD")
	if err != nil {
		return "", oops.Hint(hint).Errorf("no common ancestor between %q and HEAD (shallow clone or unrelated history)", rev)
	}
	return strings.TrimSpace(string(out)), nil
}

// ChangesSince lists what differs between the merge base of rev and HEAD and
// the working tree: committed, staged and unstaged changes plus untracked
// files that are not ignored, with renames detected. Paths are relative to dir,
// which may be any directory inside the repository; files outside it are left out.
func (g Git) ChangesSince(dir, rev string) ([]Change, error) {
	base, err := g.MergeBase(dir, rev)
	if err != nil {
		return nil, err
	}
	changes, err := g.diffChanges(dir, base)
	if err != nil {
		return nil, err
	}
	others, _, err := g.run(dir, nil, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, oops.Wrapf(err, "list untracked files")
	}
	known := map[string]bool{}
	for _, c := range changes {
		known[c.Path] = true
	}
	for _, p := range strings.Split(string(others), "\x00") {
		if p != "" && !known[p] {
			changes = append(changes, Change{Path: p, Status: 'A', AllAdded: true})
		}
	}
	sortChanges(changes)
	return changes, nil
}

// StagedChanges lists what is staged: the difference between HEAD and the index.
func (g Git) StagedChanges(dir string) ([]Change, error) {
	if !g.IsRepo(dir) {
		return nil, oops.Errorf("%s is not inside a git repository", dir)
	}
	if _, _, err := g.run(dir, nil, "rev-parse", "--verify", "--quiet", "HEAD^{commit}"); err != nil {
		return nil, oops.Hint("Make an initial commit, or run with --all.").Errorf("the repository has no commits to compare the index with")
	}
	changes, err := g.diffChanges(dir, "--cached")
	if err != nil {
		return nil, err
	}
	sortChanges(changes)
	return changes, nil
}

// diffChanges runs the name-status and the zero-context patch diff against
// target (a revision or --cached) and joins them on the new path.
func (g Git) diffChanges(dir, target string) ([]Change, error) {
	// Explicit prefixes and config overrides keep the +++ header parseable
	// whatever diff.noprefix, diff.mnemonicPrefix or diff.src/dstPrefix say.
	common := []string{
		"-c", "diff.noprefix=false", "-c", "diff.mnemonicPrefix=false", "-c", "color.diff=false",
		"diff", "--relative", "-M", "--no-ext-diff", "--no-textconv", "--no-color", "--src-prefix=a/", "--dst-prefix=b/",
	}
	targetArgs := []string{target, "--"}
	if target == "--cached" {
		targetArgs = []string{"--cached", "HEAD", "--"}
	}
	nameArgs := append(append(append([]string{}, common...), "--name-status", "-z"), targetArgs...)
	names, _, err := g.run(dir, nil, nameArgs...)
	if err != nil {
		return nil, oops.Wrapf(err, "list changed files")
	}
	patchArgs := append(append(append([]string{}, common...), "-U0"), targetArgs...)
	patch, _, err := g.run(dir, nil, patchArgs...)
	if err != nil {
		return nil, oops.Wrapf(err, "read the diff")
	}
	changes := parseNameStatus(names)
	added := parsePatchAdded(patch)
	for i := range changes {
		c := &changes[i]
		if c.Status == 'A' {
			c.AllAdded = true
			continue
		}
		c.Added = added[c.Path]
	}
	return changes, nil
}

func parseNameStatus(out []byte) []Change {
	fields := strings.Split(string(out), "\x00")
	var changes []Change
	for i := 0; i < len(fields); i++ {
		status := fields[i]
		if status == "" {
			continue
		}
		switch status[0] {
		case 'R', 'C':
			if i+2 >= len(fields) {
				return changes
			}
			changes = append(changes, Change{Path: fields[i+2], OldPath: fields[i+1], Status: 'R'})
			if status[0] == 'C' {
				changes[len(changes)-1].Status = 'A'
			}
			i += 2
		default:
			if i+1 >= len(fields) {
				return changes
			}
			st := status[0]
			if st != 'A' && st != 'D' {
				st = 'M'
			}
			changes = append(changes, Change{Path: fields[i+1], Status: st})
			i++
		}
	}
	return changes
}

// parsePatchAdded maps each new path in a -U0 patch to the line ranges added.
func parsePatchAdded(patch []byte) map[string][]LineRange {
	out := map[string][]LineRange{}
	sc := bufio.NewScanner(bytes.NewReader(patch))
	sc.Buffer(make([]byte, 0, 64*1024), 16<<20)
	current := ""
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "+++ "):
			current = patchPath(strings.TrimPrefix(line, "+++ "))
		case strings.HasPrefix(line, "@@ ") && current != "":
			if r, ok := hunkAdded(line); ok {
				out[current] = append(out[current], r)
			}
		}
	}
	return out
}

func patchPath(s string) string {
	s = strings.TrimSuffix(s, "\t")
	if strings.HasPrefix(s, `"`) {
		if u, err := strconv.Unquote(s); err == nil {
			s = u
		}
	}
	if s == "/dev/null" || !strings.HasPrefix(s, "b/") {
		return ""
	}
	return s[2:]
}

// hunkAdded reads the new side of "@@ -a,b +c,d @@".
func hunkAdded(header string) (LineRange, bool) {
	rest := strings.TrimPrefix(header, "@@ ")
	_, plus, found := strings.Cut(rest, " +")
	if !found {
		return LineRange{}, false
	}
	spec, _, _ := strings.Cut(plus, " ")
	startS, countS, hasCount := strings.Cut(spec, ",")
	start, err := strconv.Atoi(startS)
	if err != nil {
		return LineRange{}, false
	}
	count := 1
	if hasCount {
		if count, err = strconv.Atoi(countS); err != nil {
			return LineRange{}, false
		}
	}
	if count <= 0 {
		return LineRange{}, false
	}
	return LineRange{Start: start, End: start + count - 1}, true
}

func sortChanges(c []Change) { sort.Slice(c, func(i, j int) bool { return c[i].Path < c[j].Path }) }
