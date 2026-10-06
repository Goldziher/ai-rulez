package gitutil

import (
	"strconv"
	"strings"

	"github.com/samber/oops"
)

// ChangesBetween lists what the commit head changed relative to base (for a
// merged change its first parent and the merge): renames detected, new-side
// added line ranges per file. The working tree is not read.
func (g Git) ChangesBetween(dir, base, head string) ([]Change, error) {
	for _, r := range [][2]string{{"base", base}, {"head", head}} {
		if err := CheckArg(r[0], r[1]); err != nil {
			return nil, oops.Wrap(err)
		}
	}
	if !g.IsRepo(dir) {
		return nil, oops.Errorf("%s is not inside a repository", dir)
	}
	changes, err := g.diffChangesArgs(dir, []string{base, head, "--"})
	if err != nil {
		return nil, err
	}
	sortChanges(changes)
	return changes, nil
}

// Commit is one commit of a first-parent history.
type Commit struct {
	SHA string
	// Parent is the first parent ("" for a root commit).
	Parent  string
	Subject string
}

// FirstParentCommits lists the newest n commits on the first-parent line of
// HEAD, newest first: what was merged into the branch, one entry per merge
// commit, squash commit or direct commit.
func (g Git) FirstParentCommits(dir string, n int) ([]Commit, error) {
	if n <= 0 {
		return nil, nil
	}
	if !g.IsRepo(dir) {
		return nil, oops.Errorf("%s is not inside a repository", dir)
	}
	out, _, err := g.run(dir, nil, "log", "--first-parent", "-n", strconv.Itoa(n), "-z", "--format=%H%x1f%P%x1f%s", "HEAD")
	if err != nil {
		return nil, oops.Wrapf(err, "list the first-parent history")
	}
	var commits []Commit
	for _, rec := range strings.Split(string(out), "\x00") {
		fields := strings.SplitN(rec, "\x1f", 3)
		if len(fields) != 3 || fields[0] == "" {
			continue
		}
		parent, _, _ := strings.Cut(fields[1], " ")
		commits = append(commits, Commit{SHA: fields[0], Parent: parent, Subject: fields[2]})
	}
	return commits, nil
}
