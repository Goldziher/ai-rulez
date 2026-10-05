package importer

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"syscall"

	"github.com/Goldziher/ai-rulez/internal/generator"
)

const (
	maxFileBytes  = 2 << 20
	maxTotalBytes = 64 << 20
)

var errSkipped = errors.New("skipped")

// reader reads from a source tree and enforces the input limits: no symlinks,
// a per-file size cap and a total cap. Every refusal is returned as an error
// whose text is the reason recorded in the report.
type reader struct {
	fsys  fs.FS
	total int64
	// problems are paths that could not be inspected or were refused (a
	// permission error, a symlink), reported as dropped findings; a path that is
	// simply absent is not a problem.
	problems map[string]string
	// generated holds the paths a previous `ai-rulez generate` run wrote, from
	// its manifest; they are output, not source.
	generated map[string]bool
}

func newReader(fsys fs.FS) *reader { return &reader{fsys: fsys} }

// note records why a path was not read. Absence is not recorded.
func (r *reader) note(p string, err error) {
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
		return
	}
	r.noteReason(p, "could not be read: "+err.Error())
}

func (r *reader) noteReason(p, reason string) {
	if r.problems == nil {
		r.problems = map[string]string{}
	}
	if _, seen := r.problems[p]; !seen {
		r.problems[p] = reason
	}
}

// flushProblems turns the recorded problems into dropped findings, in path order.
func (r *reader) flushProblems(p *Plan) {
	paths := make([]string, 0, len(r.problems))
	for pth := range r.problems {
		paths = append(paths, pth)
	}
	sort.Strings(paths)
	for _, pth := range paths {
		p.add(newFinding(StatusDropped, pth, "", "", r.problems[pth]))
	}
	r.problems = nil
}

// symlinkIn reports whether p or one of its parent directories is a symlink,
// and which component it was.
func (r *reader) symlinkIn(p string) (bool, error) {
	link, _, err := r.symlinkAt(p)
	return link, err
}

func (r *reader) symlinkAt(p string) (link bool, at string, err error) {
	parts := strings.Split(p, "/")
	for i := range parts {
		at = strings.Join(parts[:i+1], "/")
		info, lerr := fs.Lstat(r.fsys, at)
		if lerr != nil {
			return false, at, lerr
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return true, at, nil
		}
	}
	return false, p, nil
}

// exists reports whether p is a regular path inside the tree (not through a
// symlink). A path that exists but cannot be inspected is recorded as a problem.
func (r *reader) exists(p string) (isDir, ok bool) {
	link, at, err := r.symlinkAt(p)
	if err != nil {
		r.note(at, err)
		return false, false
	}
	if link {
		r.noteReason(at, "symlinks are not followed")
		return false, false
	}
	info, err := fs.Stat(r.fsys, p)
	if err != nil {
		r.note(p, err)
		return false, false
	}
	return info.IsDir(), true
}

func (r *reader) read(p string) ([]byte, error) {
	if !fs.ValidPath(p) {
		return nil, fmt.Errorf("%w: path escapes the source directory", errSkipped)
	}
	link, err := r.symlinkIn(p)
	if err != nil {
		return nil, err
	}
	if link {
		return nil, fmt.Errorf("%w: symlinks are not followed", errSkipped)
	}
	info, err := fs.Stat(r.fsys, p)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: not a regular file", errSkipped)
	}
	if info.Size() > maxFileBytes {
		return nil, fmt.Errorf("%w: file is larger than the %d MiB limit", errSkipped, maxFileBytes>>20)
	}
	if r.total+info.Size() > maxTotalBytes {
		return nil, fmt.Errorf("%w: total input exceeds the %d MiB limit", errSkipped, maxTotalBytes>>20)
	}
	data, err := fs.ReadFile(r.fsys, p)
	if err != nil {
		return nil, err
	}
	r.total += int64(len(data))
	return data, nil
}

// skipReason returns the human reason of a skipped read, or "" for other errors.
func skipReason(err error) string {
	if !errors.Is(err, errSkipped) {
		return ""
	}
	return strings.TrimPrefix(err.Error(), errSkipped.Error()+": ")
}

// dirEntries lists a directory, sorted, without symlink entries; skipped
// entries are reported through onSkip.
func (r *reader) dirEntries(dir string, onSkip func(name, reason string)) []fs.DirEntry {
	link, at, err := r.symlinkAt(dir)
	if err != nil {
		r.note(at, err)
		return nil
	}
	if link {
		r.noteReason(at, "symlinks are not followed")
		return nil
	}
	entries, err := fs.ReadDir(r.fsys, dir)
	if err != nil {
		r.noteReason(dir, "could not be read: "+err.Error())
		return nil
	}
	var out []fs.DirEntry
	for _, e := range entries {
		if e.Type()&fs.ModeSymlink != 0 {
			onSkip(path.Join(dir, e.Name()), "symlinks are not followed")
			continue
		}
		out = append(out, e)
	}
	return out
}

// walkFiles returns every regular file below dir (slash paths relative to dir),
// sorted, skipping .git and node_modules.
func (r *reader) walkFiles(dir string, onSkip func(name, reason string)) []string {
	var out []string
	var rec func(rel string)
	rec = func(rel string) {
		full := dir
		if rel != "" {
			full = path.Join(dir, rel)
		}
		for _, e := range r.dirEntries(full, onSkip) {
			name := e.Name()
			if e.IsDir() {
				if name == ".git" || name == "node_modules" {
					continue
				}
				rec(path.Join(rel, name))
				continue
			}
			out = append(out, path.Join(rel, name))
		}
	}
	rec("")
	return out
}

// loadGeneratedManifests reads the generated-file manifests of the project, if
// any. A manifest that cannot be read is skipped: the header checks still apply.
func (r *reader) loadGeneratedManifests() {
	r.generated = map[string]bool{}
	for _, name := range generator.GeneratedManifestNames() {
		data, err := r.read(DefaultConfigDir + "/" + name)
		if err != nil {
			continue
		}
		var doc struct {
			Files []string `json:"files"`
		}
		if json.Unmarshal(data, &doc) != nil {
			continue
		}
		for _, f := range doc.Files {
			r.generated[path.Clean(f)] = true
		}
	}
}
