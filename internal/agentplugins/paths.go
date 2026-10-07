package agentplugins

import (
	"path"
	"slices"
	"strings"
)

// Fixed component locations (§6.1).
const (
	manifestFile = "plugin.json"
	mcpFile      = "mcp.json"
	skillsDir    = "skills"
	skillFile    = "SKILL.md"
)

func reservedRoot(name string) bool {
	return name == manifestFile || name == mcpFile || name == skillsDir
}

// validRelPath reports whether p is a clean, relative slash path that stays
// inside its root: no empty, "." or ".." segments, no backslash, no leading
// slash.
func validRelPath(p string) bool {
	if p == "" || strings.ContainsAny(p, "\\\x00") || path.IsAbs(p) {
		return false
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
	}
	return true
}

// within reports whether rel, a slash path relative to some root, still
// resolves inside that root after lexical cleaning. "" and "." are the root.
func within(rel string) bool {
	if strings.ContainsAny(rel, "\\\x00") || path.IsAbs(rel) {
		return false
	}
	c := path.Clean(rel)
	return c != ".." && !strings.HasPrefix(c, "../")
}

// ValidNamespace reports whether ns is a reverse-domain extension namespace:
// two or more dot-separated DNS labels of lowercase letters, digits and
// interior hyphens, and not a fixed component location.
func ValidNamespace(ns string) bool {
	if reservedRoot(ns) || len(ns) > 253 {
		return false
	}
	labels := strings.Split(ns, ".")
	if len(labels) < 2 {
		return false
	}
	for _, l := range labels {
		if !validLabel(l) {
			return false
		}
	}
	return true
}

func validLabel(l string) bool {
	if l == "" || len(l) > 63 || !isLowerAlnum(l[0]) || !isLowerAlnum(l[len(l)-1]) {
		return false
	}
	for i := 0; i < len(l); i++ {
		if !isLowerAlnum(l[i]) && l[i] != '-' {
			return false
		}
	}
	return true
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
