package generator

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/gitutil"
)

// Read-only inspection helpers for `ai-rulez doctor`. None of them writes.

// UnignoredOutputs renders every output in memory and returns, sorted, the
// ignore patterns generate wants in .gitignore (committed outputs when gitignore
// is on, machine-local and secret outputs always) that git does not ignore now.
// Outside a git repository, or when git cannot answer, it returns nil.
func (g *Generator) UnignoredOutputs(profile string) ([]string, error) {
	generateMu.Lock()
	defer generateMu.Unlock()
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
	probes := make([]string, len(patterns))
	for i, pattern := range patterns {
		probes[i] = gitignoreProbe(pattern)
	}
	ignored, err := gitutil.IgnoredAmong(g.config.BaseDir, probes)
	if err != nil || ignored == nil {
		return nil, nil //nolint:nilerr // git cannot answer: report nothing rather than guess
	}
	var missing []string
	for i, pattern := range patterns {
		if !ignored[filepath.ToSlash(probes[i])] {
			missing = append(missing, pattern)
		}
	}
	return missing, nil
}

// MergedDocumentPaths lists, sorted and absolute, the shared settings documents
// earlier generate runs merged into, as recorded in the committed and the
// machine-local manifests. Unlike a render it still works when one of them no
// longer parses.
func (g *Generator) MergedDocumentPaths() []string {
	generateMu.Lock()
	defer generateMu.Unlock()
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
	generateMu.Lock()
	g.beginRun()
	var rels []string
	rels = append(rels, g.previousManifestFiles()...)
	for _, manifest := range []string{g.manifestPath(), g.localManifestPath()} {
		for rel := range g.readManifest(manifest).Merged {
			rels = append(rels, rel)
		}
	}
	g.resetRunState()
	generateMu.Unlock()

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
				if _, ok := os.LookupEnv(name); ok {
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
