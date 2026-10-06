package govview

import (
	"archive/tar"
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
	maxGitStderr        = 64 << 10
)

var commitRE = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)

// RevisionSnapshot says what ExtractRevision wrote.
type RevisionSnapshot struct {
	// Commit is the commit the revision resolved to.
	Commit string
	Files  int
	// Symlinks lists tracked symlinks that were not materialised: a snapshot never
	// holds one, because following it could leave the directory.
	Symlinks []string
}

// ExtractRevision writes the tracked files of relPath (a slash path relative to
// the root of the repository that contains dir) as they were at rev into dest,
// keeping the path: dest/relPath/... Untracked, ignored and machine-local files
// are not part of a revision, so they are absent. Nothing is fetched and the
// working tree is not touched; the files are read with `git archive`, extracted
// through an os.Root, and bounded in count and size.
func ExtractRevision(ctx context.Context, dir, rev, relPath, dest string) (*RevisionSnapshot, error) {
	if err := gitutil.CheckArg("revision", rev); err != nil {
		return nil, oops.Wrap(err)
	}
	rel := path.Clean(filepath.ToSlash(relPath))
	if rel == "." || !fs.ValidPath(rel) {
		return nil, oops.Errorf("invalid path %q for a revision snapshot", relPath)
	}
	top := gitutil.Git{}.TopLevel(dir)
	if top == "" {
		return nil, oops.With("dir", dir).Errorf("%s is not inside a git work tree: cannot read revision %q", dir, rev)
	}
	ctx, cancel := context.WithTimeout(ctx, snapshotTimeout)
	defer cancel()

	commit, err := resolveCommit(ctx, top, rev)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(dest)
	if err != nil {
		return nil, oops.With("dest", dest).Wrapf(err, "open snapshot directory")
	}
	defer root.Close() //nolint:errcheck // nothing buffered

	cmd := gitutil.Command(ctx, top, "archive", "--format=tar", commit, "--", rel)
	var stderr bytes.Buffer
	cmd.Stderr = &limitedWriter{w: &stderr, left: maxGitStderr}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, oops.Wrapf(err, "read git archive")
	}
	if err := cmd.Start(); err != nil {
		return nil, oops.Wrapf(err, "run git archive")
	}
	snap := &RevisionSnapshot{Commit: commit}
	extractErr := extractTar(stdout, rel, root, snap)
	_, _ = io.Copy(io.Discard, stdout) //nolint:errcheck // let git finish after an early stop
	waitErr := cmd.Wait()
	if extractErr != nil {
		return nil, extractErr
	}
	if waitErr != nil {
		if strings.Contains(stderr.String(), "did not match any files") {
			return nil, oops.With("rev", rev).Errorf("%s has no files at revision %q", rel, rev)
		}
		return nil, oops.With("rev", rev).Wrapf(waitErr, "git archive failed: %s", strings.TrimSpace(stderr.String()))
	}
	if snap.Files == 0 {
		return nil, oops.With("rev", rev).Errorf("%s has no files at revision %q", rel, rev)
	}
	return snap, nil
}

func resolveCommit(ctx context.Context, top, rev string) (string, error) {
	cmd := gitutil.Command(ctx, top, "rev-parse", "--verify", "--quiet", "--end-of-options", rev+"^{commit}")
	out, err := cmd.Output()
	commit := strings.TrimSpace(string(out))
	if err != nil || !commitRE.MatchString(commit) {
		return "", oops.Hint("check `git rev-parse --verify "+rev+"`; a shallow clone may lack the revision").
			Errorf("revision %q does not exist in this repository", rev)
	}
	return commit, nil
}

type limitedWriter struct {
	w    io.Writer
	left int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	n := len(p)
	if l.left <= 0 {
		return n, nil
	}
	if len(p) > l.left {
		p = p[:l.left]
	}
	l.left -= len(p)
	_, err := l.w.Write(p)
	return n, err //nolint:wrapcheck // a bytes.Buffer never fails
}

func extractTar(r io.Reader, rel string, root *os.Root, snap *RevisionSnapshot) error {
	tr := tar.NewReader(r)
	var total int64
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return oops.Wrapf(err, "read git archive")
		}
		name := path.Clean(hdr.Name)
		switch hdr.Typeflag {
		case tar.TypeXGlobalHeader:
			continue
		case tar.TypeSymlink, tar.TypeLink:
			snap.Symlinks = append(snap.Symlinks, name)
			continue
		case tar.TypeDir:
			if !fs.ValidPath(name) || name == "." {
				continue
			}
			if err := root.MkdirAll(name, snapshotDirMode); err != nil {
				return oops.With("path", name).Wrapf(err, "create snapshot directory")
			}
			continue
		case tar.TypeReg:
		default:
			continue // devices and fifos have no place in a configuration directory
		}
		if !fs.ValidPath(name) || (name != rel && !strings.HasPrefix(name, rel+"/")) {
			return oops.With("path", hdr.Name).Errorf("git archive produced %q outside %s", hdr.Name, rel)
		}
		snap.Files++
		total += hdr.Size
		if snap.Files > snapshotMaxFiles || hdr.Size > snapshotMaxFileSize || total > snapshotMaxTotal {
			return oops.Errorf("%s is too large for a revision snapshot (limit %d files, %d MiB)", rel, snapshotMaxFiles, snapshotMaxTotal>>20)
		}
		if err := writeSnapshotFile(root, name, tr); err != nil {
			return err
		}
	}
}

func writeSnapshotFile(root *os.Root, name string, r io.Reader) error {
	if dir := path.Dir(name); dir != "." {
		if err := root.MkdirAll(dir, snapshotDirMode); err != nil {
			return oops.With("path", name).Wrapf(err, "create snapshot directory")
		}
	}
	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, snapshotFileMode)
	if err != nil {
		return oops.With("path", name).Wrapf(err, "create snapshot file")
	}
	_, copyErr := io.Copy(f, io.LimitReader(r, snapshotMaxFileSize))
	closeErr := f.Close()
	return oops.Wrapf(errors.Join(copyErr, closeErr), "write snapshot file %s", name)
}
