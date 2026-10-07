package llm

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

// CacheVersion is bumped when the on-disk entry format or key derivation changes.
const CacheVersion = 2

// maxCacheEntryBytes bounds how much of one entry is read; a larger file is a
// miss and is removed. It sits above the response cap so a valid reply fits.
const maxCacheEntryBytes = maxResponseBytes + 1<<20

// Cache is a content-addressed on-disk response cache. Entries are keyed by
// the endpoint identity, model, the request content and the request's
// PromptVersion, so a changed prompt, model or version misses. Entries hold model
// output only, never keys.
//
// Every entry is authenticated: the file holds the payload and an HMAC-SHA256
// over the identity, the key and the payload, computed with a per-user secret
// kept outside the repository (see loadOrCreateSecret). A modified, truncated,
// oversized, symlinked or planted entry fails the check, counts as a miss and is
// removed. A Cache without a secret never hits and never stores.
type Cache struct {
	dir      string
	identity string
	secret   []byte
}

// NewCache returns a cache rooted at dir for the given endpoint identity (see
// cacheIdentity) authenticated with secret. A nil or empty secret disables it.
func NewCache(dir, identity string, secret []byte) *Cache {
	return &Cache{dir: dir, identity: identity, secret: secret}
}

// Dir returns the cache directory.
func (c *Cache) Dir() string { return c.dir }

// entry is the on-disk envelope.
type entry struct {
	V       int             `json:"v"`
	MAC     string          `json:"mac"`
	Payload json.RawMessage `json:"payload"`
}

// key is the cache key of a request. ok is false when the request cannot be encoded (a NaN
// temperature): such a request is never cached, rather than every one of them sharing a key.
func (c *Cache) key(kind, model string, req any) (key string, ok bool) {
	payload := struct {
		V        int    `json:"v"`
		Kind     string `json:"kind"`
		Provider string `json:"provider"`
		Model    string `json:"model"`
		Request  any    `json:"request"`
	}{CacheVersion, kind, c.identity, model, req}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", false
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), true
}

func (c *Cache) path(key string) string {
	return filepath.Join(c.dir, key[:2], key+".json")
}

func (c *Cache) mac(key string, payload []byte) string {
	m := hmac.New(sha256.New, c.secret)
	for _, part := range [][]byte{[]byte("ai-rulez llm cache v2"), []byte(c.identity), []byte(key), payload} {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len(part)))
		m.Write(n[:])
		m.Write(part)
	}
	return hex.EncodeToString(m.Sum(nil))
}

func (c *Cache) load(key string, dst any) bool {
	if len(c.secret) == 0 {
		return false
	}
	p := c.path(key)
	fi, err := os.Lstat(p)
	if err != nil {
		return false
	}
	if !fi.Mode().IsRegular() || fi.Size() > maxCacheEntryBytes {
		os.Remove(p) //nolint:errcheck,gosec // a planted symlink or oversized file is dropped, never followed
		return false
	}
	b, err := readCapped(p)
	if err != nil {
		return false
	}
	var e entry
	if json.Unmarshal(b, &e) != nil || e.V != CacheVersion ||
		!hmac.Equal([]byte(e.MAC), []byte(c.mac(key, e.Payload))) || json.Unmarshal(e.Payload, dst) != nil {
		os.Remove(p) //nolint:errcheck,gosec // a failed check is a miss and the entry is replaced
		return false
	}
	return true
}

func readCapped(p string) ([]byte, error) {
	f, err := os.Open(p) //nolint:gosec // path built from a hash under the cache directory
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxCacheEntryBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxCacheEntryBytes {
		return nil, errors.New("cache entry too large")
	}
	return b, nil
}

func (c *Cache) store(key string, v any) {
	if len(c.secret) == 0 {
		return
	}
	payload, err := json.Marshal(v)
	if err != nil {
		return
	}
	b, err := json.Marshal(entry{V: CacheVersion, MAC: c.mac(key, payload), Payload: payload})
	if err != nil {
		return
	}
	p := c.path(key)
	if !c.prepareDir(filepath.Dir(p)) {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".tmp-*") // mode 0600
	if err != nil {
		return
	}
	_, werr := tmp.Write(b)
	cerr := tmp.Close()
	if werr != nil || cerr != nil || os.Rename(tmp.Name(), p) != nil {
		os.Remove(tmp.Name()) //nolint:errcheck,gosec // best-effort cleanup of a temp file
	}
}

// prepareDir creates the cache root and shard directory with mode 0700 and
// refuses a root that is a symlink or not a directory.
func (c *Cache) prepareDir(shard string) bool {
	if err := os.MkdirAll(shard, 0o700); err != nil {
		return false
	}
	fi, err := os.Lstat(c.dir)
	if err != nil || !fi.IsDir() {
		return false
	}
	if fi.Mode().Perm()&0o077 != 0 {
		_ = os.Chmod(c.dir, 0o700) //nolint:errcheck // best effort on a directory we own
	}
	return true
}

// Clear removes every cached entry.
func (c *Cache) Clear() error {
	err := os.RemoveAll(c.dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// WithCache wraps next with the cache. Hits return without reaching next, so
// they consume no budget and no network.
func WithCache(next Client, c *Cache, defaultModel, defaultEmbedModel string) Client {
	if c == nil {
		return next
	}
	return &cacheClient{next: next, c: c, model: defaultModel, embedModel: defaultEmbedModel}
}

type cacheClient struct {
	next              Client
	c                 *Cache
	model, embedModel string
}

func (cc *cacheClient) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	if req.NoCache {
		return cc.next.Chat(ctx, req)
	}
	key, ok := cc.c.key("chat", firstNonEmpty(req.Model, cc.model), req)
	if !ok {
		return cc.next.Chat(ctx, req)
	}
	var hit ChatResponse
	if cc.c.load(key, &hit) && (req.AcceptReply == nil || req.AcceptReply(hit.Text) == nil) {
		hit.Cached, hit.CostUSD = true, 0
		return hit, nil
	}
	resp, err := cc.next.Chat(ctx, req)
	if err != nil {
		return resp, err
	}
	if req.AcceptReply == nil || req.AcceptReply(resp.Text) == nil {
		cc.c.store(key, resp)
	}
	return resp, nil
}

func (cc *cacheClient) Embed(ctx context.Context, req EmbedRequest) (EmbedResponse, error) {
	if req.NoCache {
		return cc.next.Embed(ctx, req)
	}
	key, ok := cc.c.key("embed", firstNonEmpty(req.Model, cc.embedModel), req)
	if !ok {
		return cc.next.Embed(ctx, req)
	}
	var hit EmbedResponse
	if cc.c.load(key, &hit) {
		hit.Cached, hit.CostUSD = true, 0
		return hit, nil
	}
	resp, err := cc.next.Embed(ctx, req)
	if err != nil {
		return resp, err
	}
	cc.c.store(key, resp)
	return resp, nil
}

func (cc *cacheClient) Close() error { return cc.next.Close() }
