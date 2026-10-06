package publish

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/samber/oops"
)

// Modes every archive entry is normalised to.
const (
	modeFile       = 0o644
	modeExecutable = 0o755
)

// File is one bundle file: its slash path relative to the bundle root and its bytes.
type File struct {
	Path       string
	Data       []byte
	Executable bool
}

// ValidPath reports whether p is a clean, relative, slash-separated path that
// stays inside the bundle: no empty or dot segments, no "..", no backslash,
// no NUL, no drive letter.
func ValidPath(p string) bool {
	if p == "" || strings.HasPrefix(p, "/") || strings.ContainsAny(p, "\\\x00") || path.Clean(p) != p {
		return false
	}
	if len(p) >= 2 && p[1] == ':' {
		return false
	}
	return p != "." && p != ".." && !strings.HasPrefix(p, "../")
}

// BuildArchive writes files as a reproducible tar.gz: entries sorted bytewise
// by path, every mtime the given Unix time, uid and gid 0 with no names, modes
// 0644 or 0755, regular files only (no directory entries, no PAX records) and a
// gzip header without name or time. The same files and mtime give the same
// bytes on every OS and under any umask, because nothing is read from the
// filesystem here.
func BuildArchive(files []File, mtime int64) ([]byte, error) {
	if mtime < 0 {
		mtime = 0
	}
	sorted := append([]File(nil), files...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return nil, oops.Wrapf(err, "create gzip writer")
	}
	tw := tar.NewWriter(zw)
	stamp := time.Unix(mtime, 0).UTC()
	for i, f := range sorted {
		if !ValidPath(f.Path) {
			return nil, newError(CodeBundleUnsafe, ExitFailed, "", "unsafe bundle path %q", f.Path)
		}
		if i > 0 && sorted[i-1].Path == f.Path {
			return nil, newError(CodeBundleUnsafe, ExitFailed, "", "duplicate bundle path %q", f.Path)
		}
		mode := int64(modeFile)
		if f.Executable {
			mode = modeExecutable
		}
		hdr := &tar.Header{
			Typeflag: tar.TypeReg, Name: f.Path, Size: int64(len(f.Data)), Mode: mode,
			ModTime: stamp, Format: tar.FormatGNU,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return nil, oops.With("path", f.Path).Wrapf(err, "write archive header")
		}
		if _, err := tw.Write(f.Data); err != nil {
			return nil, oops.With("path", f.Path).Wrapf(err, "write archive entry")
		}
	}
	if err := tw.Close(); err != nil {
		return nil, oops.Wrapf(err, "finish tar")
	}
	if err := zw.Close(); err != nil {
		return nil, oops.Wrapf(err, "finish gzip")
	}
	return buf.Bytes(), nil
}
