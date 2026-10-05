package llm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// CacheVersion is bumped when the on-disk entry format or key derivation changes.
const CacheVersion = 1

// Cache is a content-addressed on-disk response cache. Entries are keyed by
// provider, model, the request content and the request's PromptVersion, so a
// changed prompt, model or version misses. Entries hold model output only, never
// keys. The directory is machine-local (under .ai-rulez/local/, git-ignored).
type Cache struct {
	dir      string
	provider string
}

// NewCache returns a cache rooted at dir for the given provider identity (the
// provider name plus base URL, so two gateways never share answers).
func NewCache(dir, provider string) *Cache { return &Cache{dir: dir, provider: provider} }

// Dir returns the cache directory.
func (c *Cache) Dir() string { return c.dir }

func (c *Cache) key(kind, model string, req any) string {
	h := sha256.New()
	payload := struct {
		V        int    `json:"v"`
		Kind     string `json:"kind"`
		Provider string `json:"provider"`
		Model    string `json:"model"`
		Request  any    `json:"request"`
	}{CacheVersion, kind, c.provider, model, req}
	h.Write([]byte(mustJSON(payload)))
	return hex.EncodeToString(h.Sum(nil))
}

func (c *Cache) path(key string) string {
	return filepath.Join(c.dir, key[:2], key+".json")
}

func (c *Cache) load(key string, dst any) bool {
	b, err := os.ReadFile(c.path(key))
	if err != nil {
		return false
	}
	return json.Unmarshal(b, dst) == nil // a corrupt entry is a miss
}

func (c *Cache) store(key string, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	p := c.path(key)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".tmp-*")
	if err != nil {
		return
	}
	_, werr := tmp.Write(b)
	cerr := tmp.Close()
	if werr != nil || cerr != nil || os.Rename(tmp.Name(), p) != nil {
		os.Remove(tmp.Name()) //nolint:errcheck,gosec // best-effort cleanup of a temp file
	}
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
	key := cc.c.key("chat", firstNonEmpty(req.Model, cc.model), req)
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
	key := cc.c.key("embed", firstNonEmpty(req.Model, cc.embedModel), req)
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
