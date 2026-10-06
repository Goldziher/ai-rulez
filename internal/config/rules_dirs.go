package config

import (
	"slices"
	"strings"
	"sync"

	"github.com/Goldziher/ai-rulez/v5/internal/generator/providers/builtin"
)

// RulesDirs lists the native rules folders ai-rulez writes one file per rule
// into. Users hand-write their own rule files in the same folders, so the
// generator treats them as shared: generated files are gitignored per file and
// an existing unmanaged file is never overwritten.
//
// The list lives in config, which both the generator and the rule-file planner
// already import, so neither needs to import the other. The folders of the
// declarative built-in presets (split rules outputs) are added from the specs
// embedded in internal/generator/providers/builtin.
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
	builtinRulesDirsOnce sync.Once
	builtinRulesDirs     []string
)

// baseRulesDirs is RulesDirs plus the rules folders of the embedded provider
// specs, derived once from embedded data.
func baseRulesDirs() []string {
	builtinRulesDirsOnce.Do(func() {
		dirs := slices.Clone(RulesDirs[:])
		for _, spec := range builtin.Summaries() {
			if dir, ok := normalizeRulesDir(spec.RulesDir); ok && !slices.Contains(dirs, dir) {
				dirs = append(dirs, dir)
			}
		}
		builtinRulesDirs = dirs
	})
	return builtinRulesDirs
}

// RulesDirSet is the set of rules folders of one project: the built-in ones plus
// the folders custom provider specs write rule files into. The custom folders come
// from the project's own config, so they belong to its Config and are never shared
// with another project in the same process. A nil *RulesDirSet is the built-in set.
type RulesDirSet struct {
	mu    sync.RWMutex
	extra []string
}

// Add makes dir a rules folder of this project, so that files a custom provider
// spec writes there (outputs.rules with split = true) get the same protections
// as the built-in folders: the overwrite guard, hashes in the banner and per-file
// gitignore entries. Adding the same folder twice is a no-op.
func (s *RulesDirSet) Add(dir string) {
	dir, ok := normalizeRulesDir(dir)
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !slices.Contains(baseRulesDirs(), dir) && !slices.Contains(s.extra, dir) {
		s.extra = append(s.extra, dir)
	}
}

func (s *RulesDirSet) all() []string {
	base := baseRulesDirs()
	if s == nil {
		return base
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.extra) == 0 {
		return base
	}
	return append(slices.Clone(base), s.extra...)
}

func normalizeRulesDir(dir string) (string, bool) {
	dir = strings.TrimPrefix(strings.ReplaceAll(dir, "\\", "/"), "./")
	dir = strings.Trim(dir, "/")
	if dir == "" || dir == "." {
		return "", false
	}
	return dir + "/", true
}

// Remainder reports whether relPath is a rules folder, or something inside one,
// at the repo root or nested under a subproject. rest is the path relative to the
// rules folder ("" for the folder itself).
func (s *RulesDirSet) Remainder(relPath string) (rest string, ok bool) {
	relPath = strings.TrimPrefix(strings.ReplaceAll(relPath, "\\", "/"), "./")
	for _, dir := range s.all() {
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

// In reports whether relPath is a file inside a rules folder.
func (s *RulesDirSet) In(relPath string) bool {
	rest, ok := s.Remainder(relPath)
	return ok && rest != ""
}

// RulesDirRemainder is Remainder for the built-in rules folders only; use
// Config.RulesDirRemainder where a project's custom folders matter.
func RulesDirRemainder(relPath string) (rest string, ok bool) {
	return (*RulesDirSet)(nil).Remainder(relPath)
}

// InRulesDir is In for the built-in rules folders only; use Config.InRulesDir
// where a project's custom folders matter.
func InRulesDir(relPath string) bool {
	return (*RulesDirSet)(nil).In(relPath)
}

// AddRulesDir makes dir a rules folder of this project (see RulesDirSet.Add).
func (c *Config) AddRulesDir(dir string) {
	if c.RulesDirs == nil {
		c.RulesDirs = &RulesDirSet{}
	}
	c.RulesDirs.Add(dir)
}

// InRulesDir reports whether relPath is a file inside a rules folder of this project.
func (c *Config) InRulesDir(relPath string) bool {
	return c.rulesDirs().In(relPath)
}

// RulesDirRemainder is RulesDirSet.Remainder for this project's folders.
func (c *Config) RulesDirRemainder(relPath string) (rest string, ok bool) {
	return c.rulesDirs().Remainder(relPath)
}

func (c *Config) rulesDirs() *RulesDirSet {
	if c == nil {
		return nil
	}
	return c.RulesDirs
}
