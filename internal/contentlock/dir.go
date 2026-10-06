package contentlock

import (
	"crypto/sha256"
	"encoding/binary"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/samber/oops"
)

// Tree kinds of fetched or cached directories. Each is its own domain, so the
// digest of an include never equals the digest of a skill source with the same
// files.
const (
	KindInclude        = "include"
	KindOKFInclude     = "okf-include"
	KindInstalledSkill = "installed-skill"
	KindSkillSource    = "skill-source"
)

// Cache bookkeeping that ai-rulez keeps at the root of a cached clone (and its
// atomic-write temp file). It is not content. Only these exact names at the tree
// root are left out: a file with the same name deeper in the tree, or a longer
// name that merely starts with it, is authored content and stays pinned.
const (
	cacheMetaFile    = ".cache_meta.json"
	cacheMetaTmpFile = cacheMetaFile + ".tmp"
)

// streamChunk is the read buffer size of the streaming hash. A variable so the
// tests can force chunk boundaries inside a CRLF pair.
var streamChunk = 64 * 1024

type treeFile struct {
	rel  string // "/"-separated, relative to the root
	abs  string
	info os.FileInfo
}

// readTree returns the regular files below dir: VCS metadata (.git below the
// root) and the cache bookkeeping files at the root are left out, so the digest
// of a fresh clone equals the digest of the same tree re-read later. A symlinked
// root is refused (WalkDir does not follow it, so every such tree would share
// one constant digest) and so is any symlink inside the tree: its target is not
// pinned by the digest, so skipping it silently would let the target change, or
// be read by a loader, outside the lock.
//
// keep, when not nil, limits the tree to the top-level entries it names; a
// symlink outside them is not examined.
func readTree(dir string, keep map[string]bool) ([]treeFile, error) {
	if info, err := os.Lstat(dir); err != nil {
		return nil, oops.With("dir", dir).Wrapf(err, "digest directory")
	} else if !info.IsDir() {
		return nil, oops.With("dir", dir).Errorf("digest directory: %s is not a real directory (a symlink is not followed)", dir)
	}
	var files []treeFile
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err //nolint:wrapcheck // wrapped by the caller
		}
		rel = filepath.ToSlash(rel)
		if keep != nil && rel != "." && !strings.Contains(rel, "/") && !keep[rel] {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return oops.With("path", rel).Errorf("%s is a symlink; symlinks cannot be pinned, replace it with the file itself", rel)
		}
		if d.IsDir() {
			if d.Name() == ".git" && path != dir {
				return filepath.SkipDir
			}
			return nil
		}
		if rel == cacheMetaFile || rel == cacheMetaTmpFile {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err //nolint:wrapcheck // wrapped by the caller
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		files = append(files, treeFile{rel: rel, abs: path, info: info})
		return nil
	})
	if err != nil {
		return nil, oops.With("dir", dir).Wrapf(err, "digest directory")
	}
	return files, nil
}

// DigestDir returns the digest of the regular files below dir under the one
// hashing scheme of the lock (TreeDigest over the files, kind being one of the
// Kind* tree kinds). Modes are normalized like every other pin (fileMode).
// Files are streamed, never read whole into memory, so a large tree costs a
// constant amount of memory.
func DigestDir(kind, dir string) (string, error) {
	return digestTree(kind, dir, nil)
}

// digestTree is DigestDir limited to the top-level entries in keep (nil: all).
func digestTree(kind, dir string, keep map[string]bool) (string, error) {
	files, err := readTree(dir, keep)
	if err != nil {
		return "", err
	}
	entries := make([]leafSum, len(files))
	for i, f := range files {
		mode := fileMode(f.abs, f.info)
		sum, err := streamLeafDigest(f.abs, f.rel, mode)
		if err != nil {
			return "", oops.With("dir", dir).With("path", f.rel).Wrapf(err, "digest directory")
		}
		entries[i] = leafSum{path: f.rel, mode: mode, sum: sum}
	}
	digest, err := combineLeaves(kind, entries)
	if err != nil {
		return "", oops.With("dir", dir).Wrapf(err, "digest directory")
	}
	return digest, nil
}

// streamLeafDigest computes leafDigest of the file at abs without loading it. A
// text file needs its length after CRLF normalization before the first data
// byte is hashed (the length prefix), so it is read twice: once to count, once
// to hash.
func streamLeafDigest(abs, rel, mode string) ([sha256.Size]byte, error) {
	var zero [sha256.Size]byte
	text := IsTextPath(rel)

	size, err := streamedLength(abs, text)
	if err != nil {
		return zero, err
	}
	h := sha256.New()
	for _, field := range []string{label("file"), rel, mode} {
		writeLP(h, []byte(field))
	}
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(size)) //nolint:gosec // sizes are non-negative
	_, _ = h.Write(n[:])
	var written int64
	if err := streamFile(abs, text, func(b []byte) {
		written += int64(len(b))
		_, _ = h.Write(b)
	}); err != nil {
		return zero, err
	}
	if written != size {
		return zero, oops.Errorf("%s changed while it was being hashed", rel)
	}
	var out [sha256.Size]byte
	copy(out[:], h.Sum(nil))
	return out, nil
}

func writeLP(w io.Writer, b []byte) {
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(len(b)))
	_, _ = w.Write(n[:])
	_, _ = w.Write(b)
}

func streamedLength(abs string, text bool) (int64, error) {
	var total int64
	err := streamFile(abs, text, func(b []byte) { total += int64(len(b)) })
	return total, err
}

// streamFile feeds the bytes of the file to emit in chunks, applying the CRLF to
// LF normalization of NormalizeText when text is true (a lone CR is kept, a CR
// at a chunk boundary is held back until the next byte is known).
func streamFile(abs string, text bool, emit func([]byte)) error {
	f, err := os.Open(abs) //nolint:gosec // path comes from WalkDir below a trusted cache directory
	if err != nil {
		return err //nolint:wrapcheck // wrapped by the caller
	}
	defer f.Close() //nolint:errcheck // read-only
	buf := make([]byte, streamChunk)
	out := make([]byte, 0, streamChunk+1)
	pendingCR := false
	for {
		n, rerr := f.Read(buf)
		switch {
		case n > 0 && !text:
			emit(buf[:n])
		case n > 0:
			out, pendingCR = normalizeChunk(out[:0], buf[:n], pendingCR)
			if len(out) > 0 {
				emit(out)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr //nolint:wrapcheck // wrapped by the caller
		}
	}
	if pendingCR {
		emit([]byte{'\r'})
	}
	return nil
}

// normalizeChunk appends chunk to out with CRLF turned into LF. pendingCR says
// the previous chunk ended in a CR that is not yet written; the returned flag is
// the same for this chunk.
func normalizeChunk(out, chunk []byte, pendingCR bool) ([]byte, bool) {
	for _, c := range chunk {
		if pendingCR {
			pendingCR = false
			if c != '\n' {
				out = append(out, '\r')
			}
		}
		if c == '\r' {
			pendingCR = true
			continue
		}
		out = append(out, c)
	}
	return out, pendingCR
}
