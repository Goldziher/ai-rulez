package config

import (
	"maps"
	"slices"
)

// Clone returns a deep copy of the server. The generator resolves ${VAR}
// placeholders and ${PROJECT_ROOT} in place on the servers it holds, so the
// map built from MCPServersRaw must own its data: the as-written servers stay
// untouched and a digest of them never sees a resolved secret.
func (m MCPServer) Clone() MCPServer {
	m.Args = slices.Clone(m.Args)
	m.Env = maps.Clone(m.Env)
	m.Headers = maps.Clone(m.Headers)
	m.Profiles = slices.Clone(m.Profiles)
	m.SecretEnvKeys = slices.Clone(m.SecretEnvKeys)
	m.SecretHeaderKeys = slices.Clone(m.SecretHeaderKeys)
	m.EnvRefs = maps.Clone(m.EnvRefs)
	m.HeaderRefs = maps.Clone(m.HeaderRefs)
	if m.Enabled != nil {
		enabled := *m.Enabled
		m.Enabled = &enabled
	}
	return m
}
