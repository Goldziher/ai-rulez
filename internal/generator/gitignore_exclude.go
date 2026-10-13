package generator

import (
	"path"
	"regexp"
	"strings"
)

// dropExcludedPatterns removes from paths the entries a gitignore_exclude entry
// names: outputs the project commits on purpose. Machine-local and secret
// patterns are never dropped (committing them would leak local setup or
// credentials); an exclude entry that only reaches those is warned about.
func (g *Generator) dropExcludedPatterns(paths map[string]bool, protected map[string]bool) {
	for _, entry := range g.config.GitignoreExclude {
		matched := false
		kept := false
		for pattern := range paths {
			if !gitignoreExcludes(entry, pattern) {
				continue
			}
			matched = true
			if g.isProtectedPattern(pattern, protected) {
				kept = true
				continue
			}
			delete(paths, pattern)
		}
		if matched && kept {
			g.warnOnce("gitignore_exclude entry "+entry+" does not apply to machine-local or secret outputs: they stay ignored",
				"entry", entry)
		}
	}
}

// gitignoreExcludes reports whether a gitignore_exclude entry names the managed
// ignore pattern. Both use gitignore notation: an entry without a slash
// ("AGENTS.md", "*.md") matches the last path segment at any depth, one with a
// slash (leading or inside) is anchored at the root, "**" crosses directories, a trailing "/" names a
// directory, and an entry for a directory covers what is below it.
func gitignoreExcludes(entry, pattern string) bool {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return false
	}
	eDir := strings.HasSuffix(entry, "/")
	body := strings.Trim(strings.TrimPrefix(entry, "./"), "/")
	pDir := strings.HasSuffix(pattern, "/")
	pBody := strings.Trim(strings.TrimPrefix(pattern, "./"), "/")
	if body == "" || pBody == "" {
		return false
	}
	anchored := strings.HasPrefix(strings.TrimPrefix(entry, "./"), "/")
	if !anchored && !strings.Contains(body, "/") {
		return (globMatch(body, path.Base(pBody)) && (pDir || !eDir)) || coversAncestor(body, pBody)
	}
	re := globRegexp(body)
	segs := strings.Split(pBody, "/")
	for i := range segs {
		prefix := strings.Join(segs[:i+1], "/")
		whole := i == len(segs)-1
		if whole && eDir && !pDir {
			continue
		}
		if re.MatchString(prefix) {
			return true
		}
	}
	return false
}

// coversAncestor reports whether a slash-less entry names a directory above the
// last segment of pBody (an entry "skills/" covers ".claude/skills/x").
func coversAncestor(body, pBody string) bool {
	segs := strings.Split(pBody, "/")
	for _, seg := range segs[:len(segs)-1] {
		if globMatch(body, seg) {
			return true
		}
	}
	return false
}

func globMatch(glob, name string) bool { return globRegexp(glob).MatchString(name) }

// globRegexp compiles a gitignore-style glob: "*" and "?" stay inside a path
// segment, "**/" matches zero or more directories, "/**" and a bare "**" match
// everything below.
func globRegexp(glob string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(glob); i++ {
		switch c := glob[i]; {
		case c == '*' && strings.HasPrefix(glob[i:], "**/"):
			b.WriteString("(?:.*/)?")
			i += 2
		case c == '*' && strings.HasPrefix(glob[i:], "**"):
			b.WriteString(".*")
			i++
		case c == '*':
			b.WriteString("[^/]*")
		case c == '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}
