package govview

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
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
	snapshotExecMode    = 0o750
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
// through an os.Root (the export-ignore and export-subst attributes are not applied),
// and bounded in count and size. A file committed executable stays executable.
func ExtractRevision(ctx context.Context, dir, rev, relPath, dest string) (*RevisionSnapshot, error) {
	rel := path.Clean(filepath.ToSlash(relPath))
	if rel == "." || !fs.ValidPath(rel) {
		return nil, oops.Errorf("invalid path %q for a revision snapshot", relPath)
	}
	return extractRevision(ctx, dir, rev, rel, dest)
}

// ExtractRevisionAll is ExtractRevision for the whole repository: every tracked
// file as it was at rev, under dest, with the same limits.
func ExtractRevisionAll(ctx context.Context, dir, rev, dest string) (*RevisionSnapshot, error) {
	return extractRevision(ctx, dir, rev, ".", dest)
}

func extractRevision(ctx context.Context, dir, rev, rel, dest string) (*RevisionSnapshot, error) {
	if err := gitutil.CheckArg("revision", rev); err != nil {
		return nil, oops.Wrap(err)
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

	snap := &RevisionSnapshot{Commit: commit}
	if err := extractTree(ctx, top, commit, rel, root, snap); err != nil {
		return nil, err
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

// treeEntry is one blob of `git ls-tree -r --long`.
type treeEntry struct {
	mode string
	oid  string
	size int64
	path string
}

// extractTree writes the blobs of rel at commit into root. It reads the tree with
// ls-tree and cat-file rather than `git archive`, because archive honours the
// export-ignore and export-subst attributes of the very revision it reads: a
// committed .gitattributes could hide or rewrite files of the snapshot.
func extractTree(ctx context.Context, top, commit, rel string, root *os.Root, snap *RevisionSnapshot) error {
	args := []string{"ls-tree", "-r", "-z", "--long", commit}
	if rel != "." {
		args = append(args, "--", rel)
	}
	ls := gitutil.Command(ctx, top, args...)
	var lsErr bytes.Buffer
	ls.Stderr = &limitedWriter{w: &lsErr, left: maxGitStderr}
	out, err := ls.Output()
	if err != nil {
		return oops.Wrapf(err, "git ls-tree failed: %s", strings.TrimSpace(lsErr.String()))
	}
	var files []treeEntry
	var total int64
	for _, rec := range strings.Split(string(out), "\x00") {
		if rec == "" {
			continue
		}
		meta, name, ok := strings.Cut(rec, "\t")
		fields := strings.Fields(meta)
		if !ok || len(fields) != 4 {
			return oops.Errorf("unexpected git ls-tree record %q", rec)
		}
		mode, typ, oid, sizeText := fields[0], fields[1], fields[2], fields[3]
		name = path.Clean(name)
		if typ != "blob" {
			continue // a submodule has no files here
		}
		if mode == "120000" {
			snap.Symlinks = append(snap.Symlinks, name)
			continue
		}
		if !fs.ValidPath(name) || (rel != "." && name != rel && !strings.HasPrefix(name, rel+"/")) {
			return oops.With("path", name).Errorf("git produced %q outside %s", name, rel)
		}
		size, convErr := strconv.ParseInt(sizeText, 10, 64)
		if convErr != nil {
			return oops.Errorf("unexpected size %q in a git ls-tree record", sizeText)
		}
		total += size
		if len(files) >= snapshotMaxFiles || size > snapshotMaxFileSize || total > snapshotMaxTotal {
			return oops.Errorf("%s is too large for a revision snapshot (limit %d files, %d MiB)", rel, snapshotMaxFiles, snapshotMaxTotal>>20)
		}
		files = append(files, treeEntry{mode: mode, oid: oid, size: size, path: name})
	}
	if len(files) == 0 {
		return nil
	}
	return writeBlobs(ctx, top, files, root, snap)
}

// writeBlobs reads the blobs through one `git cat-file --batch` and writes them.
func writeBlobs(ctx context.Context, top string, files []treeEntry, root *os.Root, snap *RevisionSnapshot) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cat := gitutil.Command(ctx, top, "cat-file", "--batch")
	var stderr bytes.Buffer
	cat.Stderr = &limitedWriter{w: &stderr, left: maxGitStderr}
	stdin, err := cat.StdinPipe()
	if err != nil {
		return oops.Wrapf(err, "run git cat-file")
	}
	stdout, err := cat.StdoutPipe()
	if err != nil {
		return oops.Wrapf(err, "run git cat-file")
	}
	if err := cat.Start(); err != nil {
		return oops.Wrapf(err, "run git cat-file")
	}
	go func() {
		defer stdin.Close() //nolint:errcheck // the reader side reports failures
		for i := range files {
			if _, werr := io.WriteString(stdin, files[i].oid+"\n"); werr != nil {
				return
			}
		}
	}()
	br := bufio.NewReader(stdout)
	var readErr error
	for i := range files {
		if readErr = copyBlob(br, &files[i], root); readErr != nil {
			break
		}
		snap.Files++
	}
	cancel()
	_ = cat.Wait() //nolint:errcheck // killed after an early stop; the read error is the one to report
	if readErr != nil {
		return oops.With("stderr", strings.TrimSpace(stderr.String())).Wrapf(readErr, "read git cat-file")
	}
	return nil
}

// copyBlob reads one `oid type size\n<content>\n` answer of cat-file --batch and
// writes the content to the snapshot.
func copyBlob(br *bufio.Reader, e *treeEntry, root *os.Root) error {
	header, err := br.ReadString('\n')
	if err != nil {
		return oops.Wrapf(err, "read header of %s", e.path)
	}
	fields := strings.Fields(header)
	if len(fields) != 3 || fields[0] != e.oid || fields[1] != "blob" {
		return oops.Errorf("unexpected cat-file answer %q for %s", strings.TrimSpace(header), e.path)
	}
	size, err := strconv.ParseInt(fields[2], 10, 64)
	if err != nil || size != e.size {
		return oops.Errorf("cat-file reports %s bytes for %s, ls-tree %d", fields[2], e.path, e.size)
	}
	mode := os.FileMode(snapshotFileMode)
	if e.mode == "100755" {
		mode = snapshotExecMode
	}
	if werr := writeSnapshotFile(root, e.path, io.LimitReader(br, size), mode); werr != nil {
		return werr
	}
	if _, err := br.Discard(1); err != nil { // the newline after the content
		return oops.Wrapf(err, "read end of %s", e.path)
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
