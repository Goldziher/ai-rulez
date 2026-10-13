package lint

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// CodePluginSelectorUnmatched warns that a selector of a plugin definition
// ([[plugins_authoring]] or [[marketplace.plugins]]) matches no content, so the
// plugin bundles less than its author meant, or nothing and is dropped.
const CodePluginSelectorUnmatched = "AR9T1"

func registerPluginSelectors(s *ruleSet) {
	s.addRules(RuleInfo{CodePluginSelectorUnmatched, "plugin-selector-unmatched", SeverityWarning,
		"a skills, commands, agents or domains selector of a plugin definition matches nothing, so the plugin bundles less than intended (a plugin that ends up empty is dropped)"})
	s.addDocs(map[string]RuleDoc{
		CodePluginSelectorUnmatched: {
			Why:  "A plugin definition picks root content by name or glob and domains by name. A selector that matches nothing is almost always a typo or a skill that was renamed, and generate --plugin silently bundles less (or, when every selector misses, drops the plugin with a warning). Under --strict the warning fails the run.",
			Bad:  "[[plugins_authoring]] name = \"notify\" skills = [\"notfy-*\"] where no skill matches the glob",
			Good: "Fix the selector, or remove it from the definition",
		},
	})
	s.addRunCheck(checkPluginSelectors, AnalyzerPlugin)
}

// checkPluginSelectors reports (AR9T1) each plugin selector that matches no
// content of the project.
func checkPluginSelectors(r *runner) {
	for _, p := range r.cfg.PluginSelectorProblems() {
		path, line := r.pluginDefinitionLine(p.Plugin)
		r.add(CodePluginSelectorUnmatched, path, line, "%s", p.Message)
	}
}

// pluginDefinitionLine locates a plugin definition: its own plugins/<name>.toml
// when it has one, else the [[plugins_authoring]] or [[marketplace.plugins]] table
// of config.toml whose name is the plugin's (a marketplace or another table with
// the same name does not count), else line 1 of config.toml.
func (r *runner) pluginDefinitionLine(name string) (path string, line int) {
	if r.cfg.ConfigDir != "" {
		own := filepath.Join(r.cfg.ConfigDir, "plugins", name+".toml")
		if _, err := os.Stat(own); err == nil {
			abs, _ := filepath.Abs(own) //nolint:errcheck // display only
			return abs, 1
		}
	}
	nameLine := regexp.MustCompile(`^name\s*=\s*["']` + regexp.QuoteMeta(name) + `["']\s*(#.*)?$`)
	return r.configLineWhere(func(lines []string) int {
		inPluginTable := false
		for i, l := range lines {
			t := strings.TrimSpace(l)
			if strings.HasPrefix(t, "[") {
				inPluginTable = t == "[[plugins_authoring]]" || t == "[[marketplace.plugins]]"
				continue
			}
			if inPluginTable && nameLine.MatchString(t) {
				return i + 1
			}
		}
		return 0
	})
}
