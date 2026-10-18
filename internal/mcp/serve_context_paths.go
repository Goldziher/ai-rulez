package mcp

import (
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/judge"
	"github.com/Goldziher/ai-rulez/v5/internal/repoindex"
	"github.com/Goldziher/ai-rulez/v5/internal/verifiers/vspec"
)

// ruleAppliesTo reports whether an item of context applies to a file. Only a
// rule can be narrowed: `paths:` (Claude Code) holds globs relative to the
// scope directory, the one that holds `.claude`, anchored there as the harness
// reads them; a Cursor rule's `globs:` are relative to the same directory and
// also match a bare pattern such as `*.tsx` at any depth. A rule with neither
// applies everywhere in its scope. A restriction none of whose globs can be
// read does not hide the rule.
func ruleAppliesTo(it *repoindex.Item, path string) bool {
	only := it.Only
	if it.Kind != repoindex.KindRule || only == nil {
		return true
	}
	rel, inside := relativeToScope(it.Scope, path)
	if !inside {
		return false
	}
	usable := false
	for _, pattern := range only.Paths {
		g, ok := judge.CompileGlob(pattern)
		if !ok {
			continue
		}
		usable = true
		if g.Match(rel) {
			return true
		}
	}
	for _, pattern := range only.Globs {
		g, err := vspec.CompileGlob(pattern)
		if err != nil || strings.HasPrefix(strings.TrimSpace(pattern), "!") {
			continue
		}
		usable = true
		if g.Match(rel) {
			return true
		}
	}
	return !usable
}

// relativeToScope returns a project-relative path below a scope directory;
// inside is false for the scope itself and for a path outside it.
func relativeToScope(scope, path string) (rel string, inside bool) {
	if scope == repoindex.RootScope {
		return path, path != "" && path != repoindex.RootScope
	}
	rel, inside = strings.CutPrefix(path, scope+"/")
	return rel, inside && rel != ""
}
