package mcp

import (
	"os"
	"path"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/judge"
	"github.com/Goldziher/ai-rulez/v5/internal/repoindex"
	"github.com/Goldziher/ai-rulez/v5/internal/verifiers/vspec"
)

// appliesTo reports whether an item of context applies to a path, a file or a
// directory (asDir). A rule or a skill can be narrowed: `paths:` (Claude Code)
// holds globs relative to the scope directory, the one that holds `.claude`,
// anchored there as the harness reads them; a Cursor rule's `globs:` are
// relative to the same directory and also match a bare pattern such as `*.tsx`
// at any depth. An item with neither applies everywhere in its scope. For a
// directory it applies when some glob could match a path below it. A
// restriction none of whose globs can be read does not hide the item.
func appliesTo(it *repoindex.Item, query string, asDir bool) bool {
	only := it.Only
	if only == nil {
		return true
	}
	rel, inside := relativeToScope(it.Scope, query)
	switch {
	case inside:
	case asDir && query == it.Scope:
		rel = ""
	default:
		return false
	}
	usable := false
	for _, pattern := range only.Paths {
		g, ok := judge.CompileGlob(pattern)
		if !ok {
			continue
		}
		usable = true
		if globReaches(g, rel, asDir) {
			return true
		}
	}
	for _, pattern := range only.Globs {
		g, ok := cursorGlob(pattern)
		if !ok {
			continue
		}
		usable = true
		if globReaches(g, rel, asDir) {
			return true
		}
	}
	return !usable
}

// globReaches reports whether a glob matches a file, or could match a path
// below a directory.
func globReaches(g judge.Glob, rel string, asDir bool) bool {
	if asDir {
		return g.MayMatchBelow(rel)
	}
	return g.Match(rel)
}

// cursorGlob compiles a Cursor `globs:` entry, which matches at any depth when
// it has no slash. A negation or a malformed pattern is not usable.
func cursorGlob(pattern string) (judge.Glob, bool) {
	p := strings.TrimSpace(pattern)
	if strings.HasPrefix(p, "!") {
		return judge.Glob{}, false
	}
	if _, err := vspec.CompileGlob(p); err != nil {
		return judge.Glob{}, false
	}
	if !strings.Contains(p, "/") {
		p = "**/" + p
	}
	return judge.CompileGlob(p)
}

// isDir reports whether a query path is a directory: what is on disk decides,
// and a path that does not exist is a file when its last element has an extension.
func (v *repoView) isDir(p string) bool {
	if info, err := os.Stat(joinRoot(v.root, p)); err == nil {
		return info.IsDir()
	}
	return path.Ext(p) == ""
}

// relativeToScope returns a project-relative path below a scope directory;
// inside is false for the scope itself and for a path outside it.
func relativeToScope(scope, p string) (rel string, inside bool) {
	if scope == repoindex.RootScope {
		return p, p != "" && p != repoindex.RootScope
	}
	rel, inside = strings.CutPrefix(p, scope+"/")
	return rel, inside && rel != ""
}
