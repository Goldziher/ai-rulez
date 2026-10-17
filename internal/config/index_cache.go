package config

import (
	"strings"

	"github.com/samber/oops"
)

// The modes of [mcp] index_cache: where the repository index of
// `mcp --serve-skills --discover-skills` keeps its cache.
const (
	// IndexCacheUser keeps the cache in the user cache directory and falls back
	// to a gitignored directory in the project when that is not writable.
	IndexCacheUser = "user"
	// IndexCacheRepo keeps the cache in a gitignored directory in the project.
	IndexCacheRepo = "repo"
	// IndexCacheOff keeps no cache.
	IndexCacheOff = "off"
)

// ParseIndexCache normalises an index_cache value: empty is user, and the case
// and surrounding space are ignored. Any other word than user, repo or off is an
// error.
func ParseIndexCache(value string) (string, error) {
	switch mode := strings.ToLower(strings.TrimSpace(value)); mode {
	case "", IndexCacheUser:
		return IndexCacheUser, nil
	case IndexCacheRepo, IndexCacheOff:
		return mode, nil
	default:
		return "", oops.
			With("field", "mcp.index_cache").
			Hint(`Use "user" (the default), "repo" or "off"`).
			Errorf("invalid index_cache %q", value)
	}
}

// IndexCacheMode is the validated [mcp] index_cache mode, user when it is unset
// or invalid (Validate reports the latter).
func (c *Config) IndexCacheMode() string {
	if c == nil || c.MCP == nil {
		return IndexCacheUser
	}
	mode, err := ParseIndexCache(c.MCP.IndexCache)
	if err != nil {
		return IndexCacheUser
	}
	return mode
}
