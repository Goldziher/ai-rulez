package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"runtime/debug"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/indexcache"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
	"github.com/Goldziher/ai-rulez/v5/internal/repoindex"
)

// The index cache of the repository discovery (internal/indexcache): the
// summaries of the indexed files and the scan verdicts of clean content survive
// a restart. A cache entry is never a decision. The allow and deny rules run on
// every build, load_context scans the body it reads again, and a verdict is
// trusted only when the cache authenticates it, for exactly the bytes and the
// trust level it was recorded for.

const indexCacheSecretFile = "index-cache.key"

// IndexCacheReport says where the index cache is and how it was used by the
// build; the catalog resource carries it under skill_discovery.index.cache.
type IndexCacheReport struct {
	// Mode is where the cache lives: "user", "repo" or "off".
	Mode string `json:"mode"`
	// Dir is the cache directory; empty when off.
	Dir string `json:"dir,omitempty"`
	// Fallback is true when the project directory is used because the user cache
	// is not writable.
	Fallback bool `json:"fallback,omitempty"`
	// Hits and Misses count the indexed files the cache did and did not know.
	Hits   int `json:"hits"`
	Misses int `json:"misses"`
	// Entries and Verdicts count what the cache holds.
	Entries  int `json:"entries"`
	Verdicts int `json:"verdicts"`
	// Discarded says why the cache on disk was not used (corrupt, version or
	// integrity); empty when it was.
	Discarded string `json:"discarded,omitempty"`
	// Warning says why the cache is not where it was asked to be, or off.
	Warning string `json:"warning,omitempty"`
}

// indexCacheKey loads the secret that authenticates the cache, kept outside the
// cache like the serve catalog cache's. nil when there is none.
func (st *ServeSetup) indexCacheKey(create bool) (key []byte, why string) {
	secret := llm.UserSecretPath(indexCacheSecretFile)
	if st.CacheDir != "" {
		secret = filepath.Join(st.CacheDir, "."+indexCacheSecretFile)
	}
	if secret == "" {
		return nil, "there is no home directory to keep the secret that authenticates the index cache"
	}
	if _, statErr := os.Lstat(secret); statErr != nil && !create {
		return nil, "the index cache has not been used yet, so its secret does not exist"
	}
	key, err := llm.LoadSecretFile(secret)
	if err != nil {
		return nil, "could not load the secret that authenticates the index cache: " + err.Error()
	}
	return key, ""
}

// IndexCacheOptions are the options of the index cache of a project: where it
// is, what authenticates it and which scanner it was recorded for. With create
// false the secret that authenticates the cache is read but never created, for
// the read-only commands; without it the options carry no key.
func (st *ServeSetup) IndexCacheOptions(ctx context.Context, cfg *config.Config, create bool) (o indexcache.Options, why string) {
	host := ambient.Host{Log: st.Log, Clock: st.Clock}
	key, why := st.indexCacheKey(create)
	o = indexcache.Options{
		Host: host, Root: cfg.BaseDir, CacheDir: st.CacheDir, Mode: cfg.IndexCacheMode(), Key: key,
		Fingerprint: scanFingerprint(st.Version, cfg), Tree: indexcache.TreeState(ctx, host, cfg.BaseDir),
	}
	if !cfg.Contentless() {
		o.ConfigDir = cfg.ConfigDir
	}
	return o, why
}

// openIndexCache opens the cache of the project; never nil: a cache that is off
// or cannot be opened is an inert store whose location says why.
func (st *ServeSetup) openIndexCache(ctx context.Context, cfg *config.Config) (store *indexcache.Store, warning string) {
	o, why := st.IndexCacheOptions(ctx, cfg, true)
	store, err := indexcache.Open(ctx, o)
	if err != nil {
		st.logger().Warn("Could not open the index cache", "error", err.Error())
		return nil, err.Error()
	}
	if why != "" && store.Location().Warning == "" {
		return store, why
	}
	return store, store.Location().Warning
}

// scanFingerprint identifies the scanner a verdict was recorded for: the build
// and the project's [lint] settings. A verdict of another one is dropped.
func scanFingerprint(version string, cfg *config.Config) string {
	h := sha256.New()
	h.Write([]byte("ai-rulez-index-scan\x00" + version + "\x00" + buildStamp() + "\x00")) //nolint:errcheck,gosec // hash writes never fail
	if cfg != nil && cfg.Lint != nil {
		if data, err := json.Marshal(cfg.Lint); err == nil {
			h.Write(data) //nolint:errcheck,gosec // hash writes never fail
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// buildStamp names the build of the binary, so a rebuilt scanner does not trust
// the verdicts of the one before it.
func buildStamp() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	stamp := info.Main.Version
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision", "vcs.modified":
			stamp += "|" + s.Value
		}
	}
	return stamp
}

// reportOfCache describes how the build used the cache.
func reportOfCache(store *indexcache.Store, warning string) *IndexCacheReport {
	loc := store.Location()
	st := store.Stats()
	return &IndexCacheReport{
		Mode: loc.Kind, Dir: loc.Dir, Fallback: loc.Fallback, Hits: st.Hits, Misses: st.Misses,
		Entries: st.Entries, Verdicts: st.Verdicts, Discarded: st.Discarded, Warning: warning,
	}
}

// screenKnown judges an item that came from the cache without its content: the
// allow and deny rules apply as always, and a context item the scan found clean
// before, for these bytes, passes. Anything else is read and screened.
func (d *discovery) screenKnown(it repoindex.Item) (keep bool, reason string, known bool) {
	if !servedAsContext(it.Kind) {
		return true, "", true
	}
	if !d.st.Filter.allowsContext(&it) {
		return false, "", true
	}
	sum, ok := indexcache.SumOfHash(path.Base(it.Path), it.Hash)
	if !ok || !d.store.Enabled() {
		return false, "", false
	}
	adm := d.admission()
	level := adm.scanLevelOf(&CatalogSkill{Name: it.Name})
	if d.store.Clean(indexcache.VerdictKey(level, []indexcache.FileSum{sum})) {
		return true, "", true
	}
	return false, "", false
}

// saveCache writes the cache after the build and its admission.
func (d *discovery) saveCache(ctx context.Context) {
	if d == nil || !d.store.Enabled() {
		return
	}
	if err := d.store.Save(ctx); err != nil {
		d.st.logger().Warn("Could not save the index cache", "error", err.Error())
	}
	if d.report != nil && d.report.Index != nil {
		d.report.Index.Cache = reportOfCache(d.store, d.cacheWarning)
	}
}

// verdicts is the verdict cache of the build's admission; nil when the cache is
// off, so the scan runs for every skill.
func (d *discovery) verdicts() VerdictCache {
	if d == nil || !d.store.Enabled() {
		return nil
	}
	return d.store
}
