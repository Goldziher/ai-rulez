package includes

import (
	"encoding/json"
	"sync"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// fetchMemo records which include and skill sources a generate run has already
// fetched. Generation may load two views of the same project (the merged one and
// the shared baseline used by the drift guard); the memo makes the second view
// resolve a source from its cache instead of fetching it again.
//
// It deliberately stores no content: every consumer gets a tree of its own, read
// from the local cache after the one real fetch, so no consumer can observe or
// corrupt another's data. It is safe for concurrent use.
type fetchMemo struct {
	entries sync.Map // key -> *memoEntry
}

type memoEntry struct {
	once sync.Once
	err  error // outcome of the real fetch; replayed, not retried
}

var memoCreateMu sync.Mutex

// memoFor returns the memo attached to cfg, creating and attaching one first.
func memoFor(cfg *config.Config) *fetchMemo {
	memoCreateMu.Lock()
	defer memoCreateMu.Unlock()
	if m, ok := cfg.IncludeMemo.(*fetchMemo); ok {
		return m
	}
	m := &fetchMemo{}
	cfg.IncludeMemo = m
	return m
}

func (m *fetchMemo) entry(key string) *memoEntry {
	e, _ := m.entries.LoadOrStore(key, &memoEntry{})
	return e.(*memoEntry) //nolint:errcheck,forcetypeassert // only memoEntry values are stored
}

// fetchTree returns the content of a source. The first caller for a key runs
// fetch (the real, possibly networked, fetch); later callers run again, which
// must resolve from the local cache only, and get the first caller's error back
// if it failed.
func (m *fetchMemo) fetchTree(key string, fetch, again func() (*config.ContentTree, error)) (*config.ContentTree, error) {
	e := m.entry("tree:" + key)
	var tree *config.ContentTree
	first := false
	e.once.Do(func() {
		first = true
		tree, e.err = fetch()
	})
	if first || e.err != nil {
		return tree, e.err
	}
	return again()
}

// fetchSkill is fetchTree for an installed skill.
func (m *fetchMemo) fetchSkill(key string, fetch, again func() (config.ContentFile, error)) (config.ContentFile, error) {
	e := m.entry("skill:" + key)
	var file config.ContentFile
	first := false
	e.once.Do(func() {
		first = true
		file, e.err = fetch()
	})
	if first || e.err != nil {
		return file, e.err
	}
	return again()
}

// memoKey identifies one configured source. It stays in memory and is never logged.
func memoKey(baseDir string, source any, hasToken bool) string {
	data, err := json.Marshal(source)
	if err != nil {
		return ""
	}
	if hasToken {
		return baseDir + "\x00token\x00" + string(data)
	}
	return baseDir + "\x00" + string(data)
}
