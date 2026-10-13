package lint

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
		path, line := r.configLine(`name = "` + p.Plugin + `"`)
		r.add(CodePluginSelectorUnmatched, path, line, "%s", p.Message)
	}
}
