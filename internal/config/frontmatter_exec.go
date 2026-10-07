package config

import (
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Frontmatter keys that make a harness run something or connect to something:
// the hooks an agent, skill or command declares, and the MCP servers it defines.
const (
	// FrontmatterHooks is the normalized name of the hooks key.
	FrontmatterHooks = "hooks"
	// FrontmatterMCPServers is the normalized name of the mcpServers key
	// (also spelled mcp-servers and mcp_servers).
	FrontmatterMCPServers = "mcpservers"
)

// executingFrontmatterKeys is the one registry of frontmatter keys that execute
// or connect. The organization policy bounds them in imported content, and the
// renderers carry them to harnesses; both read this list, so a key a renderer
// passes through cannot be missing from the policy. Add a key here when a
// renderer starts passing a new one through.
var executingFrontmatterKeys = map[string]bool{
	FrontmatterHooks:      true,
	FrontmatterMCPServers: true,
}

// NormalizeFrontmatterKey folds a frontmatter key to its comparison form:
// lower case, without "-" or "_" (mcp-servers, mcp_servers and mcpServers are one
// key).
func NormalizeFrontmatterKey(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	return strings.NewReplacer("-", "", "_", "").Replace(key)
}

// IsExecutingFrontmatterKey reports whether key, in any spelling, declares hooks
// or MCP servers.
func IsExecutingFrontmatterKey(key string) bool {
	return executingFrontmatterKeys[NormalizeFrontmatterKey(key)]
}

// ExecutingExtraKeys lists the frontmatter keys, as spelled, that execute or
// connect (see IsExecutingFrontmatterKey), sorted.
func (m *Metadata) ExecutingExtraKeys() []string {
	return m.extraKeysNormalizedTo("")
}

// ExtraKeysFor lists the frontmatter keys, as spelled, whose normalized form is
// the given registry name (FrontmatterHooks, FrontmatterMCPServers), sorted.
func (m *Metadata) ExtraKeysFor(normalized string) []string {
	return m.extraKeysNormalizedTo(normalized)
}

func (m *Metadata) extraKeysNormalizedTo(normalized string) []string {
	if m == nil {
		return nil
	}
	seen := map[string]bool{}
	add := func(k string) {
		n := NormalizeFrontmatterKey(k)
		if (normalized == "" && executingFrontmatterKeys[n]) || (normalized != "" && n == normalized) {
			seen[k] = true
		}
	}
	for k, v := range m.Extra {
		if v != "" {
			add(k)
		}
	}
	for k := range m.extraNodes {
		add(k)
	}
	if len(seen) == 0 {
		return nil
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// WithoutExtra returns a copy of the metadata without the named extra keys. The
// receiver is untouched: loaded metadata can be shared between loads.
func (m *Metadata) WithoutExtra(keys ...string) *Metadata {
	if m == nil {
		return nil
	}
	out := *m
	out.Extra = make(map[string]string, len(m.Extra))
	for k, v := range m.Extra {
		out.Extra[k] = v
	}
	out.extraNodes = make(map[string]*yaml.Node, len(m.extraNodes))
	for k, n := range m.extraNodes {
		out.extraNodes[k] = n
	}
	for _, k := range keys {
		delete(out.Extra, k)
		delete(out.extraNodes, k)
	}
	return &out
}
