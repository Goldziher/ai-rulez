package mcp

import (
	"context"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Cache hints (ttlMs and cacheScope of the 2026-07-28 protocol). A listing of
// tools or prompts is fixed when the server starts, so a client may keep it; the
// content of a project changes under the server, so resource reads are never
// cached; the served skills change only when live reload is on.
const (
	// staticTTLMs is how long a client may keep a list that cannot change while
	// the server runs: five minutes.
	staticTTLMs = 5 * 60 * 1000
	// servedTTLMs is how long a client may keep served skills when nothing
	// reloads them: one minute.
	servedTTLMs = 60 * 1000

	scopePublic  = "public"
	scopePrivate = "private"
)

// authoringCacheable sets the cache hints of the authoring server.
func authoringCacheable(_ context.Context, req sdkmcp.Request, c *sdkmcp.Cacheable) {
	switch req.(type) {
	case *sdkmcp.ListToolsRequest, *sdkmcp.ListPromptsRequest, *sdkmcp.ListResourcesRequest, *sdkmcp.ListResourceTemplatesRequest:
		c.TTLMs, c.CacheScope = staticTTLMs, scopePublic
	case *sdkmcp.ReadResourceRequest:
		c.TTLMs, c.CacheScope = 0, scopePrivate // the project may change between two reads
	}
}

// servingCacheable sets the cache hints of the skill-serving server. live is
// whether the catalog can change while the server runs.
func (s *Server) servingCacheable(_ context.Context, req sdkmcp.Request, c *sdkmcp.Cacheable) {
	live := s.serve != nil && (s.serve.opts.Rebuild != nil || s.serve.opts.Revalidate != nil)
	switch req.(type) {
	case *sdkmcp.ListToolsRequest:
		c.TTLMs, c.CacheScope = staticTTLMs, scopePublic
	case *sdkmcp.ListResourcesRequest, *sdkmcp.ListResourceTemplatesRequest, *sdkmcp.ReadResourceRequest:
		c.CacheScope = scopePrivate // the catalog belongs to this project
		c.TTLMs = servedTTLMs
		if live {
			c.TTLMs = 0
		}
	}
}

// mergedCacheable sets the cache hints of the server that serves both surfaces
// (`mcp --serve-skills`). A listing of tools or prompts is fixed when the server
// starts; a served skill file changes only when the catalog reloads; an
// authoring resource is the live project, which may change between two reads.
func (s *Server) mergedCacheable(_ context.Context, req sdkmcp.Request, c *sdkmcp.Cacheable) {
	live := s.serve != nil && (s.serve.opts.Rebuild != nil || s.serve.opts.Revalidate != nil)
	switch r := req.(type) {
	case *sdkmcp.ListToolsRequest, *sdkmcp.ListPromptsRequest:
		c.TTLMs, c.CacheScope = staticTTLMs, scopePublic
	case *sdkmcp.ListResourcesRequest, *sdkmcp.ListResourceTemplatesRequest:
		c.CacheScope = scopePrivate // the catalog and the project belong to this project
		c.TTLMs = servedTTLMs
		if live {
			c.TTLMs = 0
		}
	case *sdkmcp.ReadResourceRequest:
		c.CacheScope = scopePrivate
		if r.Params != nil && s.servedFile(r.Params.URI) {
			c.TTLMs = servedTTLMs
			if live {
				c.TTLMs = 0
			}
		}
	}
}

// servedFile reports whether a URI is a file of a served skill.
func (s *Server) servedFile(uri string) bool {
	cat := s.cat()
	if cat == nil {
		return false
	}
	_, ok := cat.File(uri)
	return ok
}
