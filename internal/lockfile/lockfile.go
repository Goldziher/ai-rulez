// Package lockfile reads, writes and verifies ai-rulez.lock, the committed
// record of what every remote include and installed skill resolved to: the
// requested ref, the commit it pointed to, and a digest of the imported files.
// With a lock, generation is reproducible even when a remote branch moves, and a
// tampered or swapped tree fails verification instead of becoming instructions.
package lockfile

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/samber/oops"
)

// FileName is the lock file, kept in the configuration directory.
const FileName = "ai-rulez.lock"

// Version is the lock format version.
const Version = 1

// Entry kinds.
const (
	KindInclude = "include"
	KindSkill   = "skill"
	// KindSource pins one [[skill_sources]] entry: the commit its ref resolved
	// to and the digest of the fetched tree.
	KindSource = "source"
	// KindServed pins one served skill by name: Digest is the digest the skills
	// server reports (over the rendered files). [lock] enforce checks against it.
	KindServed = "served"
)

// Entry pins one remote source. Source has credentials redacted: the lock is committed.
type Entry struct {
	Name   string `toml:"name"`
	Source string `toml:"source"`
	Path   string `toml:"path,omitempty"`
	// Ref is the ref the config requested ("" when it set none, meaning HEAD).
	Ref string `toml:"ref,omitempty"`
	// Commit is the full SHA Ref resolved to when the lock was written.
	Commit string `toml:"commit"`
	// Digest is "sha256:<hex>" over the imported file tree (see DigestDir).
	Digest string `toml:"digest"`
}

// File is the parsed lock.
type File struct {
	Version int     `toml:"version"`
	Include []Entry `toml:"include,omitempty"`
	Skill   []Entry `toml:"skill,omitempty"`
	Source  []Entry `toml:"source,omitempty"`
	Served  []Entry `toml:"served,omitempty"`
}

// Path returns the lock path for a configuration directory.
func Path(configDir string) string { return filepath.Join(configDir, FileName) }

// Load reads the lock in configDir. A missing file returns (nil, nil).
func Load(configDir string) (*File, error) {
	data, err := os.ReadFile(Path(configDir))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, oops.With("path", Path(configDir)).Wrapf(err, "read lock file")
	}
	var f File
	if err := toml.Unmarshal(data, &f); err != nil {
		return nil, oops.With("path", Path(configDir)).Wrapf(err, "parse lock file")
	}
	if f.Version != Version {
		return nil, oops.With("path", Path(configDir)).
			Hint("Regenerate the lock with `ai-rulez lock`").
			Errorf("unsupported lock file version %d (want %d)", f.Version, Version)
	}
	return &f, nil
}

// Save writes the lock deterministically: entries sorted by name, no timestamps.
func Save(configDir string, f *File) error {
	out := File{Version: Version, Include: sorted(f.Include), Skill: sorted(f.Skill), Source: sorted(f.Source), Served: sorted(f.Served)}
	var buf bytes.Buffer
	buf.WriteString("# ai-rulez.lock: pins every remote include and installed skill. Commit this file.\n")
	buf.WriteString("# Refresh it with `ai-rulez lock`; `ai-rulez generate --locked` fails when it is stale.\n\n")
	enc := toml.NewEncoder(&buf)
	if err := enc.Encode(out); err != nil {
		return oops.Wrapf(err, "encode lock file")
	}
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return oops.With("dir", configDir).Wrapf(err, "create config directory")
	}
	if err := os.WriteFile(Path(configDir), buf.Bytes(), 0o644); err != nil {
		return oops.With("path", Path(configDir)).Wrapf(err, "write lock file")
	}
	return nil
}

func sorted(in []Entry) []Entry {
	out := append([]Entry(nil), in...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Find returns the entry for kind and name, or nil.
func (f *File) Find(kind, name string) *Entry {
	if f == nil {
		return nil
	}
	list := *f.list(kind)
	for i := range list {
		if list[i].Name == name {
			return &list[i]
		}
	}
	return nil
}

// list returns the entries of a kind; an unknown kind means includes.
func (f *File) list(kind string) *[]Entry {
	switch kind {
	case KindSkill:
		return &f.Skill
	case KindSource:
		return &f.Source
	case KindServed:
		return &f.Served
	}
	return &f.Include
}

// Set replaces or adds an entry of the given kind.
func (f *File) Set(kind string, e Entry) {
	list := f.list(kind)
	for i := range *list {
		if (*list)[i].Name == e.Name {
			(*list)[i] = e
			return
		}
	}
	*list = append(*list, e)
}

// Want is one configured remote source that the lock should cover. Source is
// already redacted.
type Want struct {
	Kind, Name, Source, Path, Ref string
}

// Covers reports whether e pins exactly the source the config asks for. A
// changed source, path or ref makes the pin stale.
func (e *Entry) Covers(w Want) bool {
	return e != nil && e.Source == w.Source && e.Path == w.Path && e.Ref == w.Ref
}

// IsFullSHA reports whether ref is a full 40-hex commit SHA.
func IsFullSHA(ref string) bool {
	if len(ref) != 40 {
		return false
	}
	for _, c := range ref {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// DigestDir returns "sha256:<hex>" over the regular files below dir: for each
// file in path order, its slash-separated relative path, its executable bit and
// its bytes. VCS metadata and ai-rulez cache bookkeeping are left out, so the
// digest of a fresh clone equals the digest of the same tree re-read later.
func DigestDir(dir string) (string, error) {
	h := sha256.New()
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		name := d.Name()
		if d.IsDir() {
			if name == ".git" && path != dir {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(name, ".cache_meta.json") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err //nolint:wrapcheck // wrapped below
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path) //nolint:gosec // path comes from WalkDir below a trusted cache directory
		if err != nil {
			return err //nolint:wrapcheck // wrapped below
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err //nolint:wrapcheck // wrapped below
		}
		exec := "0"
		if info.Mode().Perm()&0o111 != 0 {
			exec = "1"
		}
		h.Write([]byte(filepath.ToSlash(rel) + "\x00" + exec + "\x00"))
		sum := sha256.Sum256(data)
		h.Write(sum[:])
		return nil
	})
	if err != nil {
		return "", oops.With("dir", dir).Wrapf(err, "digest directory")
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}
