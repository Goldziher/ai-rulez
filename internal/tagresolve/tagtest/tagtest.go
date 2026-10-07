// Package tagtest builds local git repositories with tags for tests: a work
// tree and a bare remote ("file://") the code under test lists and clones. It
// needs the git binary and never touches the network.
package tagtest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/stretchr/testify/require"
)

// Repo is a work tree plus the bare repository that plays the remote. Every
// mutation is pushed to the remote, forcing tags and branches.
type Repo struct {
	t    *testing.T
	Work string
	Bare string
	// URL is the file:// URL of the remote.
	URL string
	// Date, when set (a git date such as "2020-01-02T03:04:05Z"), is the author
	// and committer date of the commits and tags made from now on, so a test can
	// build a history of old releases and a brand-new one.
	Date string
}

// fileURL returns a file URL for an absolute path; on Windows C:\x becomes file:///C:/x.
func fileURL(p string) string {
	p = filepath.ToSlash(p)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return "file://" + p
}

// New creates an empty repository on branch main.
func New(t *testing.T) *Repo {
	t.Helper()
	work := t.TempDir()
	bare := filepath.Join(t.TempDir(), "remote.git")
	r := &Repo{t: t, Work: work, Bare: bare, URL: fileURL(bare)}
	r.git(work, "init", "--quiet", "--initial-branch=main")
	r.git("", "init", "--quiet", "--bare", "--initial-branch=main", bare)
	return r
}

func (r *Repo) git(dir string, args ...string) string {
	r.t.Helper()
	cmd := gitutil.CommandNoContext(dir, args...)
	cmd.Env = append(gitutil.Env(nil),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
	)
	if r.Date != "" {
		cmd.Env = append(cmd.Env, "GIT_AUTHOR_DATE="+r.Date, "GIT_COMMITTER_DATE="+r.Date)
	}
	out, err := cmd.CombinedOutput()
	require.NoError(r.t, err, "git %v: %s", args, out)
	return strings.TrimSpace(string(out))
}

// Write creates or replaces a file of the work tree; a nil-content call is not supported.
func (r *Repo) Write(rel, content string) {
	r.t.Helper()
	p := filepath.Join(r.Work, filepath.FromSlash(rel))
	require.NoError(r.t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(r.t, os.WriteFile(p, []byte(content), 0o644))
}

// Commit commits the work tree and pushes main. It returns the commit SHA.
func (r *Repo) Commit(msg string) string {
	r.t.Helper()
	r.git(r.Work, "add", "-A")
	r.git(r.Work, "commit", "--quiet", "--allow-empty", "-m", msg)
	r.git(r.Work, "push", "--quiet", "--force", r.Bare, "main")
	return r.Head()
}

// Head is the SHA of HEAD.
func (r *Repo) Head() string { return r.git(r.Work, "rev-parse", "HEAD") }

// Tag creates (or, with force, moves) a lightweight tag at HEAD and pushes it.
func (r *Repo) Tag(name string) { r.tag(name, false) }

// AnnotatedTag creates an annotated tag at HEAD and pushes it.
func (r *Repo) AnnotatedTag(name string) { r.tag(name, true) }

func (r *Repo) tag(name string, annotated bool) {
	r.t.Helper()
	args := []string{"tag", "-f"}
	if annotated {
		args = append(args, "-a", "-m", name)
	}
	r.git(r.Work, append(args, name)...)
	r.git(r.Work, "push", "--quiet", "--force", r.Bare, "refs/tags/"+name)
}

// DeleteTag removes a tag from the work tree and the remote.
func (r *Repo) DeleteTag(name string) {
	r.t.Helper()
	r.git(r.Work, "tag", "-d", name)
	r.git(r.Work, "push", "--quiet", r.Bare, ":refs/tags/"+name)
}

// TagCommit is the commit a tag resolves to.
func (r *Repo) TagCommit(name string) string { return r.git(r.Work, "rev-parse", name+"^{commit}") }

// TagObject is the object a tag ref points to (the tag object when annotated).
func (r *Repo) TagObject(name string) string { return r.git(r.Work, "rev-parse", "refs/tags/"+name) }
