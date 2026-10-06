// Package releasetime finds when a git tag was released, for the minimum
// release age gate (`min_release_age`, docs/lockfile.md). Three sources, in order
// of trust: the forge's publish time (internal/forge), the time this machine
// first saw the tag (a local file), and the date in the git data. It implements
// tagresolve.ReleaseTimer.
package releasetime

import (
	"sort"
	"sync"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
	"github.com/Goldziher/ai-rulez/v5/internal/tagresolve"
	"github.com/pelletier/go-toml/v2"
	"github.com/samber/oops"
)

// Limits on the observation file: it is local and unauthenticated, so it is
// read with a size cap and kept bounded. maxSeenBytes is the cap safefs.ReadRegular
// enforces on read; the writer stays under maxSeenWriteBytes so a file it wrote
// is always one it can read back (a larger file used to break the load and,
// with it, the age gate).
const (
	maxSeenBytes      = 1 << 20
	maxSeenWriteBytes = maxSeenBytes - 64<<10
	maxSeenEntries    = 5000
	// maxObservePerSource bounds the tags one source can add: a repository with
	// thousands of junk tags cannot push every other source out of the record.
	maxObservePerSource = 1000
	// FileName is the observation file in the user cache directory.
	FileName = "observed-tags.toml"
)

// observation is one tag-at-a-commit this machine has seen.
type observation struct {
	Source string    `toml:"source"`
	Tag    string    `toml:"tag"`
	Commit string    `toml:"commit"`
	Seen   time.Time `toml:"seen"`
}

type seenFile struct {
	Tag []observation `toml:"tag"`
}

// SeenStore records when this machine first saw each tag (at each commit: a
// tag that moves is a new release). The file is local and uncommitted; a local
// attacker who edits it can only make a tag look older, i.e. adopt it sooner,
// which they could do by editing the config. It is safe for concurrent use.
type SeenStore struct {
	path string

	mu     sync.Mutex
	loaded bool
	err    error
	by     map[string]observation
}

// OpenSeenStore returns a store backed by path. Nothing is read until first use.
func OpenSeenStore(path string) *SeenStore { return &SeenStore{path: path} }

func seenKey(source, tag, commit string) string { return source + "\x00" + tag + "\x00" + commit }

func (s *SeenStore) load() error {
	if s.loaded {
		return s.err
	}
	s.loaded, s.by = true, map[string]observation{}
	data, err := safefs.ReadRegular(s.path)
	switch {
	case err == nil:
	case isNotExist(err):
		return nil
	default:
		s.err = oops.With("path", s.path).Wrapf(err, "read the first-seen record")
		return s.err
	}
	if len(data) > maxSeenBytes {
		s.err = oops.With("path", s.path).Errorf("the first-seen record is larger than %d bytes; delete it", maxSeenBytes)
		return s.err
	}
	var f seenFile
	if err := toml.Unmarshal(data, &f); err != nil {
		s.err = oops.With("path", s.path).Hint("Delete the file; it is rebuilt as tags are seen").Wrapf(err, "parse the first-seen record")
		return s.err
	}
	for _, o := range f.Tag {
		s.by[seenKey(o.Source, o.Tag, o.Commit)] = o
	}
	return nil
}

// FirstSeen returns when source's tag at commit was first seen. A tag not seen
// before is recorded as seen now and reported as new (the age is then zero).
func (s *SeenStore) FirstSeen(source, tag, commit string, now time.Time) (seen time.Time, isNew bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.load(); err != nil {
		return time.Time{}, false, err
	}
	if o, ok := s.by[seenKey(source, tag, commit)]; ok {
		return o.Seen, false, nil
	}
	s.add(observation{Source: source, Tag: tag, Commit: commit, Seen: now.UTC()})
	return now, true, s.save()
}

// Observe records every tag of source not seen before, so a scheduled run (`lock
// --outdated` once a day) builds the history the gate reads later. It is best
// effort: a store that cannot be written is reported to the caller, not fatal.
func (s *SeenStore) Observe(source string, tags []tagresolve.RawTag, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.load(); err != nil {
		return err
	}
	changed := false
	held := 0
	for _, o := range s.by {
		if o.Source == source {
			held++
		}
	}
	for _, t := range tags {
		if held >= maxObservePerSource {
			break
		}
		if _, ok := s.by[seenKey(source, t.Name, t.Commit)]; !ok {
			held++
			s.add(observation{Source: source, Tag: t.Name, Commit: t.Commit, Seen: now.UTC()})
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return s.save()
}

func (s *SeenStore) add(o observation) { s.by[seenKey(o.Source, o.Tag, o.Commit)] = o }

// save writes the store atomically, dropping the oldest observations past the cap.
func (s *SeenStore) save() error {
	all := make([]observation, 0, len(s.by))
	for _, o := range s.by {
		all = append(all, o)
	}
	sort.Slice(all, func(i, j int) bool {
		if !all[i].Seen.Equal(all[j].Seen) {
			return all[i].Seen.After(all[j].Seen)
		}
		return seenKey(all[i].Source, all[i].Tag, all[i].Commit) < seenKey(all[j].Source, all[j].Tag, all[j].Commit)
	})
	if len(all) > maxSeenEntries {
		for _, o := range all[maxSeenEntries:] {
			delete(s.by, seenKey(o.Source, o.Tag, o.Commit))
		}
		all = all[:maxSeenEntries]
	}
	// all is newest first: drop the oldest tenth until the file fits what a reader accepts.
	var data []byte
	for {
		sorted := append([]observation(nil), all...)
		sort.Slice(sorted, func(i, j int) bool {
			a, b := sorted[i], sorted[j]
			if a.Source != b.Source {
				return a.Source < b.Source
			}
			if a.Tag != b.Tag {
				return a.Tag < b.Tag
			}
			return a.Commit < b.Commit
		})
		encoded, err := toml.Marshal(seenFile{Tag: sorted})
		if err != nil {
			return oops.Wrapf(err, "encode the first-seen record")
		}
		if len(encoded) <= maxSeenWriteBytes || len(all) <= 1 {
			data = encoded
			break
		}
		keep := len(all) - len(all)/10 - 1
		for _, o := range all[keep:] {
			delete(s.by, seenKey(o.Source, o.Tag, o.Commit))
		}
		all = all[:keep]
	}
	header := []byte("# Local record of when ai-rulez first saw each remote tag (min_release_age). Safe to delete.\n")
	if err := safefs.WriteFileAtomic(s.path, append(header, data...)); err != nil {
		return oops.With("path", s.path).Wrapf(err, "write the first-seen record")
	}
	return nil
}
