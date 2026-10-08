package catalogsite

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/samber/oops"
)

const (
	fileMode = 0o644
	dirMode  = 0o755
	// maxMarkerBytes bounds the marker read; a real one lists a few thousand paths.
	maxMarkerBytes = 8 << 20
	// maxSiteFileBytes bounds the read that verifies a file before removing it.
	maxSiteFileBytes = 256 << 20
	markerHeader     = "# ai-rulez catalog output. Files below were written by `ai-rulez catalog --html`;\n" +
		"# `--clean` removes only these. Delete this file to stop ai-rulez touching the directory.\n"
)

// WriteResult says what Write did.
type WriteResult struct {
	Written int
	Removed []string
}

// Write writes site into dir. The directory must be new, empty or marked with
// MarkerFile; anything else is refused so `--html .` can never overwrite a
// project, and a directory holding a .git or .ai-rulez folder (a project root) is
// refused outright. With clean, files a previous run wrote (listed in the marker
// with their digest) that the new site no longer has are removed; nothing else is
// ever removed: a listed path must have the shape of a site file and still hold
// the bytes this command wrote, so a hand-edited marker cannot point it at a
// project file.
//
// The marker is written before the files, listing both the new and the previously
// listed paths, so an interrupted run never leaves unmarked files behind, and
// rewritten with the final set afterwards.
//
// All access goes through an os.Root, so a symlink inside dir cannot lead a
// write or a removal outside it.
func Write(log logger.Logger, dir string, site *Site, clean bool) (*WriteResult, error) {
	if dir == "" {
		return nil, oops.Errorf("no output directory")
	}
	previous, err := prepare(dir, clean)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, oops.With("dir", dir).Wrapf(err, "open output directory")
	}
	defer root.Close() //nolint:errcheck // nothing buffered

	paths := site.Paths()
	next := make(map[string]string, len(paths))
	for _, p := range paths {
		next[p] = digestOf(site.Files[p])
	}
	// Interim marker: everything this run may write plus everything an earlier run listed.
	interim := maps.Clone(next)
	for old, sum := range previous {
		if _, ok := interim[old]; !ok {
			interim[old] = sum
		}
	}
	if err := writeMarker(root, interim); err != nil {
		return nil, err
	}
	for _, p := range paths {
		if err := writeOne(root, p, site.Files[p]); err != nil {
			return nil, err
		}
	}
	res := &WriteResult{Written: len(paths)}
	final := retireStale(log, root, previous, next, clean, res)
	if err := writeMarker(root, final); err != nil {
		return nil, err
	}
	return res, nil
}

// retireStale handles the files an earlier run listed that the new site no
// longer has: with clean it removes the ones this command still owns, and it
// returns the final marker entries (next plus every stale file left in place).
func retireStale(log logger.Logger, root *os.Root, previous, next map[string]string, clean bool,
	res *WriteResult,
) map[string]string {
	final := maps.Clone(next)
	for _, old := range slices.Sorted(maps.Keys(previous)) {
		if _, keep := next[old]; keep {
			continue
		}
		if _, statErr := root.Lstat(old); statErr != nil {
			continue // already gone
		}
		if !clean {
			final[old] = previous[old] // stays listed so a later --clean can remove it
			continue
		}
		if removeOwned(log, root, old, previous[old]) {
			res.Removed = append(res.Removed, old)
		} else {
			final[old] = previous[old]
		}
	}
	sort.Strings(res.Removed)
	return final
}

func digestOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func writeMarker(root *os.Root, entries map[string]string) error {
	var b strings.Builder
	b.WriteString(markerHeader)
	for _, p := range slices.Sorted(maps.Keys(entries)) {
		if sum := entries[p]; sum != "" {
			b.WriteString(sum + " ")
		}
		b.WriteString(p + "\n")
	}
	return writeOne(root, MarkerFile, []byte(b.String()))
}

// prepare creates dir if needed, enforces the marker rule and returns the files
// the previous run listed.
func prepare(dir string, clean bool) (map[string]string, error) {
	info, err := os.Lstat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if mkErr := os.MkdirAll(dir, dirMode); mkErr != nil {
			return nil, oops.With("dir", dir).Wrapf(mkErr, "create output directory")
		}
		return nil, nil
	case err != nil:
		return nil, oops.With("dir", dir).Wrapf(err, "inspect output directory")
	case !info.IsDir() && info.Mode()&fs.ModeSymlink == 0:
		return nil, oops.With("dir", dir).Errorf("%s is not a directory", dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, oops.With("dir", dir).Wrapf(err, "read output directory")
	}
	for _, name := range []string{".git", ".ai-rulez"} {
		if _, statErr := os.Lstat(filepath.Join(dir, name)); statErr == nil {
			return nil, oops.With("dir", dir).Hint("write the site into its own directory, such as ./site").
				Errorf("%s holds a %s folder: it is a project root, refusing to write a catalog site into it", dir, name)
		}
	}
	marker := filepath.Join(dir, MarkerFile)
	mInfo, mErr := os.Lstat(marker)
	hasMarker := mErr == nil && mInfo.Mode().IsRegular()
	if !hasMarker {
		if len(entries) > 0 {
			return nil, oops.With("dir", dir).Hint("write into a new or empty directory, or one a previous `catalog --html` run filled").
				Errorf("%s is not empty and has no %s marker: refusing to write%s", dir, MarkerFile, cleanNote(clean))
		}
		return nil, nil
	}
	data, err := os.ReadFile(marker) //nolint:gosec // the marker of the directory the user named
	if err != nil {
		return nil, oops.With("dir", dir).Wrapf(err, "read %s", MarkerFile)
	}
	if len(data) > maxMarkerBytes {
		return nil, oops.With("dir", dir).Errorf("%s is too large", MarkerFile)
	}
	return parseMarker(string(data)), nil
}

func cleanNote(clean bool) string {
	if clean {
		return " and cleaning"
	}
	return ""
}

// parseMarker returns the listed paths with the digest recorded for each ("" when
// the line has none), dropping anything that is not a plain relative path below
// the directory: the marker is data on disk, not trusted. A path listed twice
// keeps its last entry.
func parseMarker(text string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		sum := ""
		if head, tail, ok := strings.Cut(line, " "); ok && isHexDigest(head) {
			sum, line = head, strings.TrimSpace(tail)
		}
		if !safeRel(line) || line == MarkerFile {
			continue
		}
		out[line] = sum
	}
	return out
}

func isHexDigest(s string) bool {
	if len(s) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

// siteTopLevel are the files the renderer writes at the top of the site.
var siteTopLevel = map[string]bool{
	"index.html": true, "about.html": true, "lint.html": true, "lock.html": true, "mcp.html": true, "graph.html": true,
	"catalog.json": true, "robots.txt": true,
}

// isSiteFile reports whether p has the shape of a file Render produces; --clean
// removes nothing else, whatever the marker lists.
func isSiteFile(p string) bool {
	if siteTopLevel[p] {
		return true
	}
	switch {
	case strings.HasPrefix(p, "items/"), strings.HasPrefix(p, "roles/"):
		return strings.HasSuffix(p, ".html")
	case strings.HasPrefix(p, "assets/"):
		return !strings.Contains(strings.TrimPrefix(p, "assets/"), "/")
	}
	return false
}

func safeRel(p string) bool {
	if p == "" || strings.Contains(p, "\\") || strings.HasPrefix(p, "/") || path.Clean(p) != p {
		return false
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." || seg == "." || seg == "" {
			return false
		}
	}
	return fs.ValidPath(p)
}

func writeOne(root *os.Root, name string, data []byte) error {
	if dir := path.Dir(name); dir != "." {
		if err := root.MkdirAll(dir, dirMode); err != nil {
			return oops.With("path", name).Wrapf(err, "create directory")
		}
	}
	tmp := name + ".tmp"
	f, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fileMode)
	if err != nil {
		return oops.With("path", name).Wrapf(err, "create file")
	}
	_, writeErr := f.Write(data)
	chmodErr := f.Chmod(fileMode)
	closeErr := f.Close()
	if err := errors.Join(writeErr, chmodErr, closeErr); err != nil {
		_ = root.Remove(tmp) //nolint:errcheck // best-effort cleanup
		return oops.With("path", name).Wrapf(err, "write file")
	}
	if err := root.Rename(tmp, name); err != nil {
		_ = root.Remove(tmp) //nolint:errcheck // best-effort cleanup
		return oops.With("path", name).Wrapf(err, "replace file")
	}
	return nil
}

// removeOwned removes a listed file that has the shape of a site file and still
// holds the bytes this command wrote (the digest the marker recorded), and any
// directories it leaves empty; it reports whether the file was removed.
func removeOwned(log logger.Logger, root *os.Root, name, want string) bool {
	log = logger.Or(log)
	if want == "" || !isSiteFile(name) {
		log.Warn("Not removing a file the catalog marker lists: it is not a verifiable catalog output", "path", name)
		return false
	}
	f, err := root.Open(name)
	if err != nil {
		return false
	}
	data, readErr := io.ReadAll(io.LimitReader(f, maxSiteFileBytes))
	_ = f.Close() //nolint:errcheck // read-only
	if readErr != nil || digestOf(data) != want {
		log.Warn("Not removing a catalog file that changed since it was written", "path", name)
		return false
	}
	if err := root.Remove(name); err != nil {
		return false
	}
	for dir := path.Dir(name); dir != "." && dir != "/"; dir = path.Dir(dir) {
		if root.Remove(dir) != nil { // not empty: stop
			break
		}
	}
	return true
}
