// Package targetmatch implements the matching of frontmatter `targets` against
// generated outputs. It is a leaf package shared by the presets, the DSL
// providers and the rule-file planner so every output interprets a target the
// same way.
package targetmatch

import (
	"path"
	"strings"
)

// Allow reports whether an item with the given targets may appear in an
// output. An item without targets is allowed everywhere; otherwise one target
// must select the output (see Match).
func Allow(targets, presets []string, outputs ...string) bool {
	return len(targets) == 0 || Match(targets, presets, outputs...)
}

// Match reports whether any target selects the output. An output is described
// by the preset names that own it and by the paths it is known under (full
// path, base name, root file).
//
// A target selects an output when it
//   - equals one of presets, ignoring case;
//   - is "*" or "**" (everything);
//   - equals one of outputs;
//   - ends in "/" and is a directory prefix of one of outputs;
//   - ends in "/*" or "/**" and the part before is a directory prefix; or
//   - is a glob (path.Match syntax) matching one of outputs.
//
// Paths compare case-insensitively with backslashes treated as slashes, and
// leading "./" and "/" removed.
func Match(targets, presets []string, outputs ...string) bool {
	candidates := make([]string, 0, len(outputs))
	for _, o := range outputs {
		if n := Normalize(o); n != "" {
			candidates = append(candidates, n)
		}
	}
	for _, raw := range targets {
		if matchOne(raw, presets, candidates) {
			return true
		}
	}
	return false
}

// Normalize returns the canonical, lower-case, slash-separated form of a path
// or target. A trailing slash (a directory marker) is preserved.
func Normalize(raw string) string {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if raw == "" {
		return ""
	}
	dir := strings.HasSuffix(raw, "/")
	n := path.Clean(raw)
	n = strings.TrimLeft(strings.TrimPrefix(n, "./"), "/")
	if n == "." || n == "" {
		return ""
	}
	if dir {
		n += "/"
	}
	return strings.ToLower(n)
}

// InvalidGlob reports whether target contains glob characters but is not a
// valid pattern, so it can never match anything.
func InvalidGlob(target string) bool {
	n := Normalize(target)
	if !strings.ContainsAny(n, "*?[") {
		return false
	}
	_, err := path.Match(n, "")
	return err != nil
}

func matchOne(raw string, presets, candidates []string) bool {
	target := Normalize(raw)
	if target == "" {
		return false
	}
	for _, p := range presets {
		if strings.EqualFold(strings.TrimSpace(raw), p) {
			return true
		}
	}
	if target == "*" || target == "**" {
		return true
	}
	for _, suffix := range []string{"/**", "/*"} {
		if dir, ok := strings.CutSuffix(target, suffix); ok && !strings.ContainsAny(dir, "*?[") {
			target = dir + "/"
			break
		}
	}
	if prefix, isDir := strings.CutSuffix(target, "/"); isDir {
		return underDir(prefix, candidates)
	}
	return pathMatches(target, candidates)
}

func underDir(prefix string, candidates []string) bool {
	for _, c := range candidates {
		if c == prefix || strings.HasPrefix(c, prefix+"/") {
			return true
		}
	}
	return false
}

func pathMatches(target string, candidates []string) bool {
	glob := strings.ContainsAny(target, "*?[")
	for _, c := range candidates {
		if c == target {
			return true
		}
		if !glob {
			continue
		}
		if ok, err := path.Match(target, c); err == nil && ok {
			return true
		}
	}
	return false
}
