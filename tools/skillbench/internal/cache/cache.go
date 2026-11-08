// Package cache stores verdicts by sample content, so a scanner that has not changed is not rerun.
//
// What is deterministic is the verdict. Repetitions agree on it, but a finding list can drift: Cisco's
// reported one extra low-severity rule on one of 563 skills in one of three runs, with the same verdict.
// So the key names everything that can change an answer, and a run rescans a few hits, failing if a
// verdict moved and logging if only the findings did.
package cache

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"

	"github.com/Goldziher/ai-rulez/tools/skillbench/internal/scanner"
)

// Key names one cache file.
type Key struct {
	Tool    string
	Version string
	// Adapter is the tool's scanner.Tool.AdapterDigest.
	Adapter string
	// Environment covers what the tool runs on: architecture, network, and the digest of the image
	// recipe and lock file.
	Environment string
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]`)

func (k Key) file() string {
	name := k.Tool + "-" + k.Version + "-" + k.Environment + "-" + k.Adapter + ".jsonl"
	return unsafeName.ReplaceAllString(name, "_")
}

// Store is one tool's verdicts by sample digest.
type Store struct {
	path    string
	entries map[string]scanner.Verdict
	dirty   bool
}

type line struct {
	Sample string `json:"sample"`
	scanner.Verdict
}

// Open loads the store for a key. A missing file is an empty store.
func Open(dir string, k Key) (*Store, error) {
	if k.Tool == "" || k.Version == "" || k.Adapter == "" || k.Environment == "" {
		return nil, fmt.Errorf("incomplete cache key %+v", k)
	}
	s := &Store{path: filepath.Join(dir, k.file()), entries: map[string]scanner.Verdict{}}
	f, err := os.Open(s.path) //nolint:gosec // the operator names the cache directory
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open cache: %w", err)
	}
	defer func() { _ = f.Close() }()
	dec := json.NewDecoder(bufio.NewReader(f))
	for dec.More() {
		var l line
		if err := dec.Decode(&l); err != nil {
			return nil, fmt.Errorf("parse %s: %w", s.path, err)
		}
		s.entries[l.Sample] = l.Verdict
	}
	return s, nil
}

// Len counts the stored verdicts.
func (s *Store) Len() int { return len(s.entries) }

// Get returns the verdict for a sample digest. It can carry Error: see Put.
func (s *Store) Get(digest string) (scanner.Verdict, bool) {
	v, ok := s.entries[digest]
	return v, ok
}

// Put stores a verdict. An error is kept so a cached run has the same denominators as the scan it
// replays, but never replaces an answer, and a full run retries it.
func (s *Store) Put(digest string, v scanner.Verdict) {
	if old, ok := s.entries[digest]; ok && v.Error != "" && old.Error == "" {
		return
	}
	s.entries[digest] = v
	s.dirty = true
}

// Save writes the store sorted by digest, so the same verdicts give the same bytes.
func (s *Store) Save() error {
	if !s.dirty {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o750); err != nil {
		return fmt.Errorf("create cache directory: %w", err)
	}
	tmp := s.path + ".tmp"
	f, err := os.Create(tmp) //nolint:gosec // the operator names the cache directory
	if err != nil {
		return fmt.Errorf("create cache: %w", err)
	}
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	for _, digest := range slices.Sorted(maps.Keys(s.entries)) {
		if err := enc.Encode(line{Sample: digest, Verdict: s.entries[digest]}); err != nil {
			return errors.Join(fmt.Errorf("encode cache: %w", err), f.Close())
		}
	}
	if err := w.Flush(); err != nil {
		return errors.Join(fmt.Errorf("write cache: %w", err), f.Close())
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close cache: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("replace cache: %w", err)
	}
	s.dirty = false
	return nil
}

// HashDir digests a sample directory: every file's relative path and bytes, in sorted order.
func HashDir(dir string) (string, error) {
	var names []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			names = append(names, path)
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("walk sample: %w", err)
	}
	slices.Sort(names)
	h := sha256.New()
	for _, name := range names {
		rel, err := filepath.Rel(dir, name)
		if err != nil {
			return "", fmt.Errorf("relative path: %w", err)
		}
		writeField(h, []byte(filepath.ToSlash(rel)))
		if err := hashFile(h, name); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func hashFile(h hash.Hash, name string) error {
	f, err := os.Open(name) //nolint:gosec // inside the operator's sample directory
	if err != nil {
		return fmt.Errorf("open sample: %w", err)
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat sample: %w", err)
	}
	if err := binary.Write(h, binary.BigEndian, info.Size()); err != nil {
		return fmt.Errorf("hash sample: %w", err)
	}
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("hash sample: %w", err)
	}
	return nil
}

// writeField length-prefixes b, so two paths cannot run together into a third.
func writeField(h hash.Hash, b []byte) {
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(len(b)))
	_, _ = h.Write(n[:])
	_, _ = h.Write(b)
}
