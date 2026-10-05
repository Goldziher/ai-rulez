package config

import (
	"slices"
	"strings"
	"sync"
)

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
	".devin/rules/",
	".clinerules/",
	".continue/rules/",
	".agents/rules/",
	".junie/rules/",
	".github/instructions/",
}

var (
	extraRulesDirsMu sync.RWMutex
	extraRulesDirs   []string
)

// RegisterRulesDir adds a rules folder beyond the built-in list, so that files
// a custom provider spec writes there (outputs.rules with split = true) get the
// same protections: the overwrite guard, hashes in the banner and per-file
// gitignore entries. It is process-wide because those checks are pure functions
// of an output path (InRulesDir is called from gitignore, hash injection and
// the overwrite guard, none of which carry a Config), so carrying the folders
// on Config would mean threading it through all of them. The cost is a set that
// only grows by the distinct folders of loaded specs, which is small and bounded
// by the specs a process loads. Registering the same folder twice is a no-op;
// UnregisterRulesDir lets tests undo a registration.
func RegisterRulesDir(dir string) {
	dir, ok := normalizeRulesDir(dir)
	if !ok {
		return
	}
	extraRulesDirsMu.Lock()
	defer extraRulesDirsMu.Unlock()
	if !slices.Contains(RulesDirs[:], dir) && !slices.Contains(extraRulesDirs, dir) {
		extraRulesDirs = append(extraRulesDirs, dir)
	}
}

// UnregisterRulesDir removes a folder added by RegisterRulesDir. The built-in
// folders cannot be removed. It exists for tests.
func UnregisterRulesDir(dir string) {
	dir, ok := normalizeRulesDir(dir)
	if !ok {
		return
	}
	extraRulesDirsMu.Lock()
	defer extraRulesDirsMu.Unlock()
	extraRulesDirs = slices.DeleteFunc(extraRulesDirs, func(d string) bool { return d == dir })
}

func normalizeRulesDir(dir string) (string, bool) {
	dir = strings.TrimPrefix(strings.ReplaceAll(dir, "\\", "/"), "./")
	dir = strings.Trim(dir, "/")
	if dir == "" || dir == "." {
		return "", false
	}
	return dir + "/", true
}

func allRulesDirs() []string {
	extraRulesDirsMu.RLock()
	defer extraRulesDirsMu.RUnlock()
	return append(slices.Clone(RulesDirs[:]), extraRulesDirs...)
}

// RulesDirRemainder reports whether relPath is a rules folder, or something
// inside one, at the repo root or nested under a subproject. rest is the path
// relative to the rules folder ("" for the folder itself).
func RulesDirRemainder(relPath string) (rest string, ok bool) {
	relPath = strings.TrimPrefix(strings.ReplaceAll(relPath, "\\", "/"), "./")
	for _, dir := range allRulesDirs() {
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
