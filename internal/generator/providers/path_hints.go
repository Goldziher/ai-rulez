package providers

import (
	"reflect"
	"sort"
	"strings"
)

// maxPathHintLen bounds what counts as a path-looking spec string.
const maxPathHintLen = 160

// BuiltinPathHints lists every relative path or path prefix a builtin provider
// spec names: root files, per-item directories, sidecars, hook plugin modules.
// A template segment ("{id}/SKILL.md") ends the hint at the directory before it.
// The list over-approximates on purpose (any path-looking string of a spec), and
// callers use it only to decide whether a manifest entry could be an ai-rulez
// output at all, never to decide what to write.
func BuiltinPathHints() []string {
	seen := map[string]bool{}
	for _, spec := range loadBuiltinSpecs() {
		collectPathHints(reflect.ValueOf(spec), seen, 0)
	}
	hints := make([]string, 0, len(seen))
	for hint := range seen {
		hints = append(hints, hint)
	}
	sort.Strings(hints)
	return hints
}

func collectPathHints(v reflect.Value, seen map[string]bool, depth int) {
	const maxDepth = 12
	if depth > maxDepth {
		return
	}
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			collectPathHints(v.Elem(), seen, depth+1)
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).IsExported() {
				collectPathHints(v.Field(i), seen, depth+1)
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			collectPathHints(v.Index(i), seen, depth+1)
		}
	case reflect.Map:
		for _, key := range v.MapKeys() {
			collectPathHints(v.MapIndex(key), seen, depth+1)
		}
	case reflect.String:
		if hint := pathHint(v.String()); hint != "" {
			seen[hint] = true
		}
	default:
	}
}

// pathHint returns the path-like prefix of s, or "" when s does not look like a
// relative path.
func pathHint(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > maxPathHintLen || strings.ContainsAny(s, " \t\n:*?<>|\"'$\\") ||
		strings.HasPrefix(s, "/") || strings.HasPrefix(s, "~") || strings.HasPrefix(s, "..") {
		return ""
	}
	s = strings.TrimPrefix(s, "./")
	var kept []string
	for _, segment := range strings.Split(s, "/") {
		if strings.Contains(segment, "{") {
			break
		}
		kept = append(kept, segment)
	}
	hint := strings.Join(kept, "/")
	if hint == "" || (!strings.Contains(s, "/") && !strings.Contains(s, ".")) {
		return ""
	}
	return hint
}
