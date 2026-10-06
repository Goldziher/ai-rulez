package policy

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"sort"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
)

const (
	cacheSecretName = "policy-cache.key"
	cacheDirName    = "policy"
	cacheVersion    = 1
	cacheMACLabel   = "ai-rulez policy cache v1"
	tofuMACLabel    = "ai-rulez policy tofu v1"
	tofuFile        = "tofu.json"
	maxCacheMeta    = 64 << 10
)

// cache is the user-level store of fetched policies: the last good copy of each
// pinned URL, and the digests recorded by trust-on-first-use. It lives in the
// user's cache directory, outside any repository, and every entry carries an HMAC
// under a per-user secret, so a checkout (or a cache restored into one) cannot
// plant a policy: an entry that fails its MAC is a miss.
type cache struct {
	dir    string
	secret []byte
}

// cacheEntry is the metadata next to a cached body.
type cacheEntry struct {
	V         int    `json:"v"`
	URL       string `json:"url"`
	Digest    string `json:"digest"`
	FetchedAt string `json:"fetched_at"`
	BodySHA   string `json:"body_sha256"`
	MAC       string `json:"mac"`
}

// openCache returns the cache for env, or an error when there is no home
// directory or secret (a cache is optional: the caller carries on without).
func openCache(env ambient.Env) (*cache, error) {
	dir, err := config.CacheDirIn(env, cacheDirName)
	if err != nil {
		return nil, err //nolint:wrapcheck // contextual already
	}
	secret, err := llm.LoadSecretFile(llm.UserSecretPathIn(env, cacheSecretName))
	if err != nil {
		return nil, err //nolint:wrapcheck // the caller reports it as "no cache"
	}
	return &cache{dir: dir, secret: secret}, nil
}

func (c *cache) mac(label string, parts ...string) string {
	m := hmac.New(sha256.New, c.secret)
	m.Write([]byte(label))
	for _, p := range parts {
		m.Write([]byte{0})
		m.Write([]byte(p))
	}
	return hex.EncodeToString(m.Sum(nil))
}

// name is the file stem of the entry for a URL and digest.
func (c *cache) name(rawURL, digest string) string {
	sum := sha256.Sum256([]byte(rawURL + "\x00" + digest))
	return hex.EncodeToString(sum[:16])
}

func (c *cache) entryMAC(e cacheEntry) string {
	return c.mac(cacheMACLabel, e.URL, e.Digest, e.FetchedAt, e.BodySHA)
}

// put stores body for the URL and digest, fetched at the given time.
func (c *cache) put(rawURL, digest string, body []byte, at time.Time) error {
	sum := sha256.Sum256(body)
	e := cacheEntry{V: cacheVersion, URL: rawURL, Digest: digest, FetchedAt: at.UTC().Format(time.RFC3339), BodySHA: hex.EncodeToString(sum[:])}
	e.MAC = c.entryMAC(e)
	meta, err := json.Marshal(e)
	if err != nil {
		return err //nolint:wrapcheck // best-effort store
	}
	stem := filepath.Join(c.dir, c.name(rawURL, digest))
	if err := safefs.WriteFileAtomic(stem+".toml", body); err != nil {
		return err //nolint:wrapcheck // best-effort store
	}
	return safefs.WriteFileAtomic(stem+".json", meta) //nolint:wrapcheck // best-effort store
}

// get returns the cached body for the URL and digest, and when it was fetched. A
// missing, unreadable or unauthentic entry is a miss: its MAC, the body's hash
// and the pinned digest must all agree.
func (c *cache) get(rawURL, digest string) (body []byte, fetchedAt time.Time, ok bool) {
	stem := filepath.Join(c.dir, c.name(rawURL, digest))
	meta, err := safefs.ReadRegular(stem + ".json")
	if err != nil || len(meta) > maxCacheMeta {
		return nil, time.Time{}, false
	}
	var e cacheEntry
	if json.Unmarshal(meta, &e) != nil || e.V != cacheVersion || e.URL != rawURL || e.Digest != digest || !hmac.Equal([]byte(e.MAC), []byte(c.entryMAC(e))) {
		return nil, time.Time{}, false
	}
	body, err = safefs.ReadRegular(stem + ".toml")
	if err != nil || len(body) > maxPolicyBytes {
		return nil, time.Time{}, false
	}
	sum := sha256.Sum256(body)
	if hex.EncodeToString(sum[:]) != e.BodySHA || policyDigest(body) != digest {
		return nil, time.Time{}, false
	}
	at, err := time.Parse(time.RFC3339, e.FetchedAt)
	if err != nil {
		return nil, time.Time{}, false
	}
	return body, at, true
}

// tofuDoc is the record of digests accepted on first use.
type tofuDoc struct {
	V       int               `json:"v"`
	Entries map[string]string `json:"entries"`
	MAC     string            `json:"mac"`
}

func (c *cache) tofuMAC(entries map[string]string) string {
	urls := make([]string, 0, len(entries))
	for u := range entries {
		urls = append(urls, u)
	}
	sort.Strings(urls)
	parts := make([]string, 0, 2*len(urls))
	for _, u := range urls {
		parts = append(parts, u, entries[u])
	}
	return c.mac(tofuMACLabel, parts...)
}

func (c *cache) tofuPath() string { return filepath.Join(c.dir, tofuFile) }

// tofuLoad reads the record; a missing or unauthentic one is empty.
func (c *cache) tofuLoad() map[string]string {
	data, err := safefs.ReadRegular(c.tofuPath())
	if err != nil || len(data) > maxCacheMeta {
		return map[string]string{}
	}
	var d tofuDoc
	if json.Unmarshal(data, &d) != nil || d.V != cacheVersion || d.Entries == nil || !hmac.Equal([]byte(d.MAC), []byte(c.tofuMAC(d.Entries))) {
		return map[string]string{}
	}
	return d.Entries
}

// tofuGet returns the digest recorded for the URL.
func (c *cache) tofuGet(rawURL string) (string, bool) {
	d, ok := c.tofuLoad()[rawURL]
	return d, ok && digestPattern.MatchString(d)
}

// tofuRecord records the digest accepted for the URL.
func (c *cache) tofuRecord(rawURL, digest string) error {
	entries := c.tofuLoad()
	entries[rawURL] = digest
	data, err := json.Marshal(tofuDoc{V: cacheVersion, Entries: entries, MAC: c.tofuMAC(entries)})
	if err != nil {
		return err //nolint:wrapcheck // the caller reports it
	}
	return safefs.WriteFileAtomic(c.tofuPath(), data) //nolint:wrapcheck // the caller reports it
}

// policyDigest is digest, for code whose parameters shadow the name.
func policyDigest(data []byte) string { return digest(data) }
