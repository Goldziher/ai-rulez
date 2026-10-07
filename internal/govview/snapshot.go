package govview

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
	"github.com/Goldziher/ai-rulez/v5/internal/workspace"
)

// Limits on what a revision snapshot may hold, so a hostile or huge repository
// cannot fill the disk or hang the command.
const (
	snapshotTimeout     = 2 * time.Minute
	snapshotMaxFiles    = 50000
	snapshotMaxFileSize = 32 << 20
	snapshotMaxTotal    = 512 << 20
	snapshotDirMode     = 0o750
	snapshotFileMode    = 0o640
	snapshotExecMode    = 0o750
)

// snapshotLimits are the bounds of one extraction.
type snapshotLimits struct {
	files    int
	fileSize int64
	total    int64
}

var defaultSnapshotLimits = snapshotLimits{files: snapshotMaxFiles, fileSize: snapshotMaxFileSize, total: snapshotMaxTotal}

var commitRE = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)

// RevisionSnapshot says what ExtractRevision wrote.
type RevisionSnapshot struct {
	// Commit is the commit the revision resolved to.
	Commit string
	Files  int
	// Symlinks lists tracked symlinks that were not materialized: a snapshot never
	// holds one, because following it could leave the directory.
	Symlinks []string
}

// ExtractRevision writes the tracked files of relPath (a slash path relative to
// the root of the repository that contains dir) as they were at rev into dest,
// keeping the path: dest/relPath/... Untracked, ignored and machine-local files
// are not part of a revision, so they are absent. Nothing is fetched and the
// working tree is not touched. The files are read through a workspace.GitSnapshot
// of the commit (so the export-ignore and export-subst attributes are not applied
// and git runs through the runner ctx carries), written through an os.Root, and
// bounded in count and size. A file committed executable stays executable.
func ExtractRevision(ctx context.Context, dir, rev, relPath, dest string) (*RevisionSnapshot, error) {
	rel := path.Clean(filepath.ToSlash(relPath))
	if rel == "." || !fs.ValidPath(rel) {
		return nil, oops.Errorf("invalid path %q for a revision snapshot", relPath)
	}
	return extractRevision(ctx, defaultSnapshotLimits, dir, rev, rel, dest)
}

// ExtractRevisionAll is ExtractRevision for the whole repository: every tracked
// file as it was at rev, under dest, with the same limits.
func ExtractRevisionAll(ctx context.Context, dir, rev, dest string) (*RevisionSnapshot, error) {
	return extractRevision(ctx, defaultSnapshotLimits, dir, rev, ".", dest)
}

func extractRevision(ctx context.Context, lim snapshotLimits, dir, rev, rel, dest string) (*RevisionSnapshot, error) {
	if err := gitutil.CheckArg("revision", rev); err != nil {
		return nil, oops.Wrap(err)
	}
	run := runner.FromContext(ctx)
	git := gitutil.New(run)
	top := git.TopLevelContext(ctx, dir)
	if top == "" {
		return nil, oops.With("dir", dir).Errorf("%s is not inside a git work tree: cannot read revision %q", dir, rev)
	}
	ctx, cancel := context.WithTimeout(ctx, snapshotTimeout)
	defer cancel()

	commit, err := resolveCommit(ctx, git, top, rev)
	if err != nil {
		return nil, err
	}
	var paths []string
	if rel != "." {
		paths = []string{rel}
	}
	snap, err := workspace.GitSnapshot(ctx, top, commit, run, paths...)
	if err != nil {
		return nil, oops.Wrapf(err, "read revision %q", rev)
	}
	root, err := os.OpenRoot(dest)
	if err != nil {
		return nil, oops.With("dest", dest).Wrapf(err, "open snapshot directory")
	}
	defer root.Close() //nolint:errcheck // nothing buffered

	out := &RevisionSnapshot{Commit: commit}
	files, err := collectFiles(snap, lim, rel, out)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, oops.With("rev", rev).Errorf("%s has no files at revision %q", rel, rev)
	}
	if err := writeFiles(snap, files, root, out); err != nil {
		return nil, err
	}
	return out, nil
}

func resolveCommit(ctx context.Context, git gitutil.Git, top, rev string) (string, error) {
	res := git.Exec(ctx, top, nil, "rev-parse", "--verify", "--quiet", "--end-of-options", rev+"^{commit}")
	commit := strings.TrimSpace(string(res.Stdout))
	if gitutil.ResultErr(res) != nil || !commitRE.MatchString(commit) {
		return "", oops.Hint("check `git rev-parse --verify "+rev+"`; a shallow clone may lack the revision").
			Errorf("revision %q does not exist in this repository", rev)
	}
	return commit, nil
}

// snapFile is one regular file to extract.
type snapFile struct {
	name string
	exec bool
}

// collectFiles lists the regular files below rel in the snapshot and applies the
// limits from the tree metadata, before any content is read. Symlinks are
// reported in out, never listed; submodules hold no files here.
func collectFiles(snap workspace.Snapshot, lim snapshotLimits, rel string, out *RevisionSnapshot) ([]snapFile, error) {
	start, err := snap.Lstat(rel)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, oops.With("path", rel).Wrapf(err, "stat in the snapshot")
	}
	c := &fileCollector{snap: snap, lim: lim, rel: rel, out: out}
	if err := c.visit(rel, start); err != nil {
		return nil, err
	}
	return c.files, nil
}

// fileCollector is the state of one collectFiles walk.
type fileCollector struct {
	snap  workspace.Snapshot
	lim   snapshotLimits
	rel   string
	out   *RevisionSnapshot
	files []snapFile
	total int64
}

func (c *fileCollector) visit(name string, info fs.FileInfo) error {
	switch {
	case info.Mode()&fs.ModeSymlink != 0:
		c.out.Symlinks = append(c.out.Symlinks, name)
	case info.IsDir():
		return c.walk(name)
	case info.Mode().IsRegular():
		return c.add(name, info)
	}
	return nil
}

// add lists a regular file, inside rel and within the limits.
func (c *fileCollector) add(name string, info fs.FileInfo) error {
	if !fs.ValidPath(name) || (c.rel != "." && name != c.rel && !strings.HasPrefix(name, c.rel+"/")) {
		return oops.With("path", name).Errorf("the snapshot produced %q outside %s", name, c.rel)
	}
	c.total += info.Size()
	if len(c.files) >= c.lim.files || info.Size() > c.lim.fileSize || c.total > c.lim.total {
		return oops.Errorf("%s is too large for a revision snapshot (limit %d files, %d MiB)", c.rel, c.lim.files, c.lim.total>>20)
	}
	c.files = append(c.files, snapFile{name: name, exec: info.Mode()&0o111 != 0})
	return nil
}

func (c *fileCollector) walk(dir string) error {
	entries, err := c.snap.ReadDir(dir)
	if err != nil {
		return oops.With("path", dir).Wrapf(err, "list the snapshot")
	}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			return oops.With("path", e.Name()).Wrapf(err, "stat in the snapshot")
		}
		if err := c.visit(path.Join(dir, e.Name()), info); err != nil {
			return err
		}
	}
	return nil
}

// writeFiles reads the files (in batches when the snapshot can) and writes them
// below root.
func writeFiles(snap workspace.Snapshot, files []snapFile, root *os.Root, out *RevisionSnapshot) error {
	modes := make(map[string]os.FileMode, len(files))
	names := make([]string, len(files))
	for i, f := range files {
		names[i] = f.name
		modes[f.name] = snapshotFileMode
		if f.exec {
			modes[f.name] = snapshotExecMode
		}
	}
	write := func(name string, data []byte) error {
		if err := writeSnapshotFile(root, name, bytes.NewReader(data), modes[name]); err != nil {
			return err
		}
		out.Files++
		return nil
	}
	if br, ok := snap.(workspace.BatchReader); ok {
		return br.ReadBatch(names, write) //nolint:wrapcheck // already contextual
	}
	for _, name := range names {
		data, err := snap.ReadFile(name)
		if err != nil {
			return oops.With("path", name).Wrapf(err, "read from the snapshot")
		}
		if err := write(name, data); err != nil {
			return err
		}
	}
	return nil
}

func writeSnapshotFile(root *os.Root, name string, r io.Reader, mode os.FileMode) error {
	if dir := path.Dir(name); dir != "." {
		if err := root.MkdirAll(dir, snapshotDirMode); err != nil {
			return oops.With("path", name).Wrapf(err, "create snapshot directory")
		}
	}
	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return oops.With("path", name).Wrapf(err, "create snapshot file")
	}
	_, copyErr := io.Copy(f, r)
	closeErr := f.Close()
	return oops.Wrapf(errors.Join(copyErr, closeErr), "write snapshot file %s", name)
}
