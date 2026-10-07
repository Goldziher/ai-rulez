package generator

import (
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/roles"
)

// Read-only inspection helpers for `ai-rulez doctor`. None of them writes.

// UnignoredOutputs renders every output in memory and returns, sorted, the
// ignore patterns generate wants in .gitignore (committed outputs when gitignore
// is on, machine-local and secret outputs always) that git does not ignore now.
// Outside a git repository, or when git cannot answer, it returns nil.
func (g *Generator) UnignoredOutputs(profile string) ([]string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.beginRun()
	defer g.resetRunState()

	outputs, _, err := g.collectOutputs(profile)
	if err != nil {
		return nil, err
	}
	patterns := make([]string, 0)
	for pattern := range g.collectGitignorePaths(outputs) {
		patterns = append(patterns, pattern)
	}
	sort.Strings(patterns)
	probes, ranges := flattenProbes(patterns)
	ignored, err := g.git().IgnoredAmongContext(g.ctx, g.config.BaseDir, probes)
	if err != nil || ignored == nil {
		return nil, nil //nolint:nilerr // git cannot answer: report nothing rather than guess
	}
	var missing []string
	for i, pattern := range patterns {
		for _, probe := range probes[ranges[i][0]:ranges[i][1]] {
			if !ignored[filepath.ToSlash(probe)] {
				missing = append(missing, pattern)
				break
			}
		}
	}
	return missing, nil
}

// MergedDocumentPaths lists, sorted and absolute, the shared settings documents
// earlier generate runs merged into, as recorded in the committed and the
// machine-local manifests. Unlike a render it still works when one of them no
// longer parses.
func (g *Generator) MergedDocumentPaths() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.beginRun()
	defer g.resetRunState()

	seen := map[string]bool{}
	for _, manifest := range []string{g.manifestPath(), g.localManifestPath()} {
		for rel := range g.readManifest(manifest).Merged {
			// The manifest is a committed file anyone can edit: a path that climbs out
			// of the project is never a document this run may open.
			if slices.Contains(strings.Split(filepath.ToSlash(rel), "/"), "..") {
				continue
			}
			abs := filepath.Join(g.config.BaseDir, filepath.FromSlash(rel))
			if g.withinScope(abs) {
				seen[abs] = true
			}
		}
	}
	paths := make([]string, 0, len(seen))
	for p := range seen {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

// GeneratedPaths lists, sorted and absolute, every file the previous generate
// run recorded in the committed and the machine-local manifests, merged
// documents included. A watcher uses it to tell a generated output from a
// source, so a source tree that also holds outputs does not retrigger itself.
func (g *Generator) GeneratedPaths() []string {
	g.mu.Lock()
	g.beginRun()
	var rels []string
	rels = append(rels, g.previousManifestFiles()...)
	for _, manifest := range []string{g.manifestPath(), g.localManifestPath()} {
		for rel := range g.readManifest(manifest).Merged {
			rels = append(rels, rel)
		}
	}
	g.resetRunState()
	g.mu.Unlock()

	seen := map[string]bool{}
	for _, rel := range rels {
		if slices.Contains(strings.Split(filepath.ToSlash(rel), "/"), "..") {
			continue
		}
		if abs := filepath.Join(g.config.BaseDir, filepath.FromSlash(rel)); g.withinScope(abs) {
			seen[abs] = true
		}
	}
	paths := make([]string, 0, len(seen))
	for p := range seen {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

// ConfigDirOutputNames are the files a run writes directly inside the
// configuration directory: the two generation manifests and the roles manifest.
// A watcher on that directory must not treat their rewrite as a source change.
// The roles manifest is listed whether or not it is enabled or exists yet, since
// the first run creates it.
func ConfigDirOutputNames() []string {
	return []string{generatedManifestName, generatedLocalManifestName, roles.FileName}
}

// MissingMCPEnv is an MCP ${VAR} placeholder no source can resolve.
type MissingMCPEnv struct {
	Server string
	Field  string // "env" or "headers"
	Key    string
	Var    string
}

// MissingMCPEnv returns, sorted, the ${VAR} placeholders in the env and headers
// of the enabled MCP servers active in profile ("" selects the default) that neither --env overrides, the process environment,
// the dotenv files nor ${PROJECT_ROOT} can resolve. Nothing is modified.
func (g *Generator) MissingMCPEnv(profile string) ([]MissingMCPEnv, error) {
	active := g.resolveProfile(profile)
	dotenv, err := g.loadMCPDotenvValues()
	if err != nil {
		return nil, err
	}
	var missing []MissingMCPEnv
	check := func(server, field string, values map[string]string) {
		for key, value := range values {
			for _, m := range mcpEnvPlaceholderPattern.FindAllStringSubmatch(value, -1) {
				name := m[1]
				if _, ok := g.config.MCPEnvOverrides[name]; ok {
					continue
				}
				if _, ok := g.host().LookupEnv(name); ok {
					continue
				}
				if _, ok := dotenv[name]; ok {
					continue
				}
				if name == projectRootEnvName && g.config.BaseDir != "" {
					continue
				}
				missing = append(missing, MissingMCPEnv{Server: server, Field: field, Key: key, Var: name})
			}
		}
	}
	for name, server := range g.config.MCPServers {
		if server == nil || !server.IsEnabled() || !config.ProfileMatches(active, server.Profiles) {
			continue
		}
		check(name, "env", server.Env)
		check(name, "headers", server.Headers)
	}
	sort.Slice(missing, func(i, j int) bool {
		a, b := missing[i], missing[j]
		return strings.Join([]string{a.Server, a.Field, a.Key, a.Var}, "\x00") < strings.Join([]string{b.Server, b.Field, b.Key, b.Var}, "\x00")
	})
	return missing, nil
}
