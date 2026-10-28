package mcp

import (
	"os"
	"path"
	"regexp"
	"slices"
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
// restriction none of whose globs can be read does not hide the item, but one
// whose globs are lost (a frontmatter block that names `paths:` and does not
// parse, or a `paths` value that is not a string or a list of strings) does:
// where the item applies cannot be known, so it fails closed.
func appliesTo(it *repoindex.Item, query string, asDir bool) bool {
	only := it.Only
	if only == nil {
		return true
	}
	if only.Unreadable {
		return false
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
		if globReaches(judgeReach(g), rel, asDir) {
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

// reachGlob is a glob as an item's restriction uses it: whether it matches a
// file, and whether it could match a path below a directory.
type reachGlob struct {
	match, below func(string) bool
}

func judgeReach(g judge.Glob) reachGlob { return reachGlob{match: g.Match, below: g.MayMatchBelow} }

// globReaches reports whether a glob matches a file, or could match a path
// below a directory.
func globReaches(g reachGlob, rel string, asDir bool) bool {
	if asDir {
		return g.below(rel)
	}
	return g.match(rel)
}

// bracketClass is a [...] run, which Cursor's matcher (vspec) reads literally.
var bracketClass = regexp.MustCompile(`\[[^\]]*\]`)

// cursorGlob compiles a Cursor `globs:` entry the way the rest of ai-rulez reads
// it (vspec): braces alternate, an alternative without a slash matches at any
// depth, and brackets are literal characters. A file is matched exactly so. For a
// directory, whether a path below could match, each alternative is compared with
// the judge matcher, with a bracket run widened to `*` so a literal bracket never
// hides an item (the answer errs on the side of keeping it). A negation or a
// malformed pattern is not usable.
func cursorGlob(pattern string) (reachGlob, bool) {
	p := strings.TrimSpace(pattern)
	if strings.HasPrefix(p, "!") {
		return reachGlob{}, false
	}
	file, err := vspec.CompileGlob(p)
	if err != nil {
		return reachGlob{}, false
	}
	alts, err := vspec.Alternatives(p)
	if err != nil {
		return reachGlob{}, false
	}
	var below []judge.Glob
	for _, alt := range alts {
		if g, ok := judge.CompileGlob(bracketClass.ReplaceAllString(alt, "*")); ok {
			below = append(below, g)
		}
	}
	return reachGlob{match: file.Match, below: func(dir string) bool {
		return slices.ContainsFunc(below, func(g judge.Glob) bool { return g.MayMatchBelow(dir) })
	}}, true
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
