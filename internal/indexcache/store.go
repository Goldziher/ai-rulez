package indexcache

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/repoindex"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
)

// Why a cache file was discarded.
const (
	ReasonCorrupt   = "corrupt"
	ReasonVersion   = "version"
	ReasonIntegrity = "integrity"
)

const (
	// racyWindow is how close to the save an entry's modification time may be
	// for the entry to be trusted: a file written in the same timestamp tick as
	// the save may have changed again without changing its size or time.
	racyWindow = 2 * time.Second
	// keepGenerations is how many saves an entry or verdict survives unused.
	keepGenerations = 25
	// refreshAfter is how stale a used entry gets before a save refreshes it.
	refreshAfter = 5
	// maxRecords bounds the entries and the verdicts one cache holds.
	maxRecords = 200000
	// maxFileBytes bounds the index file that is read.
	maxFileBytes = 256 << 20

	lockAttempts = 1000
	lockPause    = 5 * time.Millisecond
	filePerm     = 0o600
)

type entryRec struct {
	Size  int64                `json:"s"`
	MTime int64                `json:"m"`
	Item  repoindex.CachedItem `json:"i"`
	Gen   int64                `json:"g"`
}

type payload struct {
	SavedAt     int64               `json:"saved_at"`
	Gen         int64               `json:"gen"`
	Tree        string              `json:"tree,omitempty"`
	Fingerprint string              `json:"fingerprint,omitempty"`
	Entries     map[string]entryRec `json:"entries"`
	Verdicts    map[string]int64    `json:"verdicts"`
}

type document struct {
	Version int             `json:"version"`
	MAC     string          `json:"mac"`
	Payload json.RawMessage `json:"payload"`
}

// Stats describe a store.
type Stats struct {
	// Hits and Misses count the Lookup calls.
	Hits, Misses int
	// Entries and Verdicts count what the store holds now.
	Entries, Verdicts int
	// Discarded is why the file on disk was not used (ReasonCorrupt,
	// ReasonVersion or ReasonIntegrity); empty when it was, or there was none.
	Discarded string
}

// Store is an open cache. All methods are safe for concurrent use, and every
// method of a nil Store, or of one whose cache is off, does nothing.
type Store struct {
	loc Location
	o   Options

	mu       sync.Mutex
	entries  map[string]entryRec
	verdicts map[string]int64
	touched  map[string]bool
	marked   map[string]bool
	loaded   payload
	gen      int64
	dirty    bool
	stats    Stats
	savedAt  int64
}

// Open locates the cache and loads it. A cache that cannot be trusted is
// discarded (Stats.Discarded says why) and starts empty; Open fails only when
// the location cannot be decided at all.
func Open(ctx context.Context, o Options) (*Store, error) {
	loc, err := Locate(ctx, o)
	if err != nil {
		return nil, err
	}
	s := &Store{
		loc: loc, o: o, entries: map[string]entryRec{}, verdicts: map[string]int64{},
		touched: map[string]bool{}, marked: map[string]bool{},
	}
	if loc.Kind == KindOff {
		return s, nil
	}
	p, reason := readPayload(loc.Dir, o.Key)
	s.stats.Discarded = reason
	s.dirty = reason != "" // a cache that was discarded is written again
	s.loaded = p
	s.gen = p.Gen + 1
	s.savedAt = p.SavedAt
	if p.Tree == o.Tree {
		s.entries = p.Entries
	}
	if p.Fingerprint == o.Fingerprint {
		s.verdicts = p.Verdicts
	}
	if s.entries == nil {
		s.entries = map[string]entryRec{}
	}
	if s.verdicts == nil {
		s.verdicts = map[string]int64{}
	}
	return s, nil
}

// Location is where the store keeps its cache.
func (s *Store) Location() Location {
	if s == nil {
		return Location{Kind: KindOff}
	}
	return s.loc
}

// Enabled reports whether the store keeps anything; false for nil and for a
// cache that is off.
func (s *Store) Enabled() bool { return s.active() }

// active reports whether the store keeps anything.
func (s *Store) active() bool { return s != nil && s.loc.Kind != KindOff }

// Stats reports the store's counters.
func (s *Store) Stats() Stats {
	if !s.active() {
		return Stats{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.stats
	st.Entries, st.Verdicts = len(s.entries), len(s.verdicts)
	return st
}

// Lookup implements repoindex.Cache: the summary of a file whose size and
// modification time are unchanged since the cache saw it. An entry modified
// within the racy window before the save it was loaded from is not trusted.
func (s *Store) Lookup(file string, size, mtime int64) (repoindex.CachedItem, bool) {
	if !s.active() {
		return repoindex.CachedItem{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[file]
	if !ok || e.Size != size || e.MTime != mtime || (!s.touched[file] && mtime >= s.savedAt-int64(racyWindow)) {
		s.stats.Misses++
		return repoindex.CachedItem{}, false
	}
	s.stats.Hits++
	if !s.touched[file] && e.Gen < s.gen-refreshAfter {
		e.Gen = s.gen
		s.entries[file] = e
		s.touched[file] = true
		s.dirty = true
	}
	return e.Item, true
}

// Store implements repoindex.Cache.
func (s *Store) Store(file string, size, mtime int64, item repoindex.CachedItem) {
	if !s.active() {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[file] = entryRec{Size: size, MTime: mtime, Item: item, Gen: s.gen}
	s.touched[file] = true
	s.dirty = true
}

// Clean reports whether the scan found nothing in the content a verdict key
// stands for, under the scanner this store was opened for.
func (s *Store) Clean(key string) bool {
	if !s.active() {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	gen, ok := s.verdicts[key]
	if ok && !s.marked[key] && gen < s.gen-refreshAfter {
		s.verdicts[key] = s.gen
		s.marked[key] = true
		s.dirty = true
	}
	return ok
}

// MarkClean records that the scan found nothing in the content a verdict key
// stands for.
func (s *Store) MarkClean(key string) {
	if !s.active() {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.verdicts[key] = s.gen
	s.marked[key] = true
	s.dirty = true
}

// FileSum names one scanned file and the SHA-256 of its content.
type FileSum struct {
	Name string
	Sum  [sha256.Size]byte
}

// SumOfContent is the FileSum of a file the scan has in hand.
func SumOfContent(name string, content []byte) FileSum {
	return FileSum{Name: name, Sum: sha256.Sum256(content)}
}

// SumOfHash is the FileSum of a file known only by the "sha256:<hex>" hash an
// index item carries. ok is false for any other form.
func SumOfHash(name, hash string) (FileSum, bool) {
	digest, found := strings.CutPrefix(hash, "sha256:")
	raw, err := hex.DecodeString(digest)
	if !found || err != nil || len(raw) != sha256.Size {
		return FileSum{}, false
	}
	out := FileSum{Name: name}
	copy(out.Sum[:], raw)
	return out, true
}

// VerdictKey is the key of a scan verdict: a hash of the scan level and of the
// names and content hashes of the scanned files, so a verdict stands for
// exactly those bytes.
func VerdictKey(level string, files []FileSum) string {
	h := sha256.New()
	h.Write([]byte(level))
	h.Write([]byte{0})
	for _, f := range files {
		h.Write([]byte(f.Name))
		h.Write([]byte{0})
		h.Write(f.Sum[:])
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Save writes the cache when something changed. It takes the lock file, merges
// what other servers saved since this store was opened, and replaces the index
// file by write-and-rename.
func (s *Store) Save(ctx context.Context) error {
	if !s.active() {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.dirty {
		return nil
	}
	release, err := lockDir(s.loc.Dir)
	if err != nil {
		return err
	}
	defer release()
	key := s.o.Key
	disk, _ := readPayload(s.loc.Dir, key)
	out := payload{Gen: max(disk.Gen, s.loaded.Gen) + 1, Tree: s.o.Tree, Fingerprint: s.o.Fingerprint,
		Entries: map[string]entryRec{}, Verdicts: map[string]int64{}}
	if disk.Tree == s.o.Tree {
		for k, v := range disk.Entries {
			out.Entries[k] = v
		}
	}
	if disk.Fingerprint == s.o.Fingerprint {
		for k, v := range disk.Verdicts {
			out.Verdicts[k] = v
		}
	}
	for k := range s.touched {
		e := s.entries[k]
		e.Gen = out.Gen
		out.Entries[k] = e
	}
	for k := range s.marked {
		out.Verdicts[k] = out.Gen
	}
	prune(&out)
	out.SavedAt = s.o.now().UnixNano()
	data, err := encode(out, key)
	if err != nil {
		return err
	}
	if err := gitutil.WriteFileAtomic(filepath.Join(s.loc.Dir, indexFile), data, filePerm); err != nil {
		return oops.Wrapf(err, "save the index cache")
	}
	s.dirty = false
	return nil
}

// prune drops what has not been used for keepGenerations saves, then the oldest
// records past maxRecords.
func prune(p *payload) {
	for k, e := range p.Entries {
		if e.Gen < p.Gen-keepGenerations {
			delete(p.Entries, k)
		}
	}
	for k, g := range p.Verdicts {
		if g < p.Gen-keepGenerations {
			delete(p.Verdicts, k)
		}
	}
	if len(p.Entries) > maxRecords {
		keys := make([]string, 0, len(p.Entries))
		for k := range p.Entries {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return p.Entries[keys[i]].Gen > p.Entries[keys[j]].Gen })
		for _, k := range keys[maxRecords:] {
			delete(p.Entries, k)
		}
	}
	if len(p.Verdicts) > maxRecords {
		keys := make([]string, 0, len(p.Verdicts))
		for k := range p.Verdicts {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return p.Verdicts[keys[i]] > p.Verdicts[keys[j]] })
		for _, k := range keys[maxRecords:] {
			delete(p.Verdicts, k)
		}
	}
}

func encode(p payload, key []byte) ([]byte, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return nil, oops.Wrapf(err, "encode the index cache")
	}
	data, err := json.Marshal(document{Version: FormatVersion, MAC: mac(key, body), Payload: body})
	if err != nil {
		return nil, oops.Wrapf(err, "encode the index cache")
	}
	return data, nil
}

func mac(key, body []byte) string {
	h := hmac.New(sha256.New, key)
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

// readPayload loads the cache file of dir, authenticated with key; reason is
// why it was not used ("" for a missing file or a good one).
func readPayload(dir string, key []byte) (p payload, reason string) {
	path := filepath.Join(dir, indexFile)
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return payload{}, ""
		}
		return payload{}, ReasonCorrupt
	}
	if !info.Mode().IsRegular() || info.Size() > maxFileBytes {
		return payload{}, ReasonCorrupt
	}
	data, err := os.ReadFile(path) //nolint:gosec // the cache directory is ours and the file was just checked to be regular
	if err != nil {
		return payload{}, ReasonCorrupt
	}
	var doc document
	if err := json.Unmarshal(data, &doc); err != nil {
		return payload{}, ReasonCorrupt
	}
	if doc.Version != FormatVersion {
		return payload{}, ReasonVersion
	}
	want, err := hex.DecodeString(doc.MAC)
	if len(key) == 0 || err != nil {
		return payload{}, ReasonIntegrity
	}
	sum, err := hex.DecodeString(mac(key, doc.Payload))
	if err != nil || !hmac.Equal(want, sum) {
		return payload{}, ReasonIntegrity
	}
	if err := json.Unmarshal(doc.Payload, &p); err != nil {
		return payload{}, ReasonCorrupt
	}
	return p, ""
}

// lockDir takes the lock file of a cache directory, waiting a few seconds for
// another server that holds it.
func lockDir(dir string) (release func(), err error) {
	f, err := safefs.OpenLockFile(filepath.Join(dir, lockFile))
	if err != nil {
		return nil, oops.With("dir", dir).Wrapf(err, "open the index cache lock")
	}
	for range lockAttempts {
		held, lockErr := tryLock(f)
		if lockErr != nil {
			_ = f.Close() //nolint:errcheck // already failing
			return nil, oops.With("dir", dir).Wrapf(lockErr, "lock the index cache")
		}
		if held {
			return func() {
				unlock(f)
				_ = f.Close() //nolint:errcheck // nothing to do on failure
			}, nil
		}
		time.Sleep(lockPause)
	}
	_ = f.Close() //nolint:errcheck // not held
	return nil, oops.With("dir", dir).Hint("Another ai-rulez server holds the index cache lock").Errorf("the index cache is locked")
}

// compile-time check that Store is a repoindex.Cache.
var _ repoindex.Cache = (*Store)(nil)
