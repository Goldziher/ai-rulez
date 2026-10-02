package config

import "strings"

// RulesDirs lists the native rules folders ai-rulez writes one file per rule
// into. Users hand-write their own rule files in the same folders, so the
// generator treats them as shared: generated files are gitignored per file and
// an existing unmanaged file is never overwritten.
//
// The list lives in config, which both the generator and the rule-file planner
// already import, so neither needs to import the other.
var RulesDirs = [...]string{
	".claude/rules/",
	".cursor/rules/",
	".windsurf/rules/",
	".clinerules/",
	".continue/rules/",
	".agents/rules/",
	".junie/rules/",
	".github/instructions/",
}

// RulesDirRemainder reports whether relPath is a rules folder, or something
// inside one, at the repo root or nested under a subproject. rest is the path
// relative to the rules folder ("" for the folder itself).
func RulesDirRemainder(relPath string) (rest string, ok bool) {
	relPath = strings.TrimPrefix(strings.ReplaceAll(relPath, "\\", "/"), "./")
	for _, dir := range RulesDirs {
		if strings.HasPrefix(relPath, dir) {
			return strings.TrimPrefix(relPath, dir), true
		}
		trimmed := strings.TrimSuffix(dir, "/")
		if relPath == trimmed || strings.HasSuffix(relPath, "/"+trimmed) {
			return "", true
		}
		if idx := strings.Index(relPath, "/"+dir); idx >= 0 {
			return relPath[idx+1+len(dir):], true
		}
	}
	return "", false
}

// InRulesDir reports whether relPath is a file inside a rules folder.
func InRulesDir(relPath string) bool {
	rest, ok := RulesDirRemainder(relPath)
	return ok && rest != ""
}
