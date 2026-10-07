package config

import (
	"github.com/pelletier/go-toml/v2"
	"github.com/samber/oops"
)

// Two lint tables look alike and mean different things: [lint.tolerate] maps a
// rule to a tolerated finding count, [lint.budgets.<kind>] sets the size limits
// of a content kind. The checks below turn the obvious mix-ups into one
// sentence naming the right table.

const (
	lintToleratePath = "[lint.tolerate]"
	lintBudgetsPath  = "[lint.budgets.<kind>]"
)

// swappedLintTablesTOML explains a TOML document that fails to decode because a
// lint table holds the other table's shape. It returns nil when the document is
// not that kind of mistake.
func swappedLintTablesTOML(path string, data []byte) error {
	var doc map[string]any
	if toml.Unmarshal(data, &doc) != nil {
		return nil
	}
	return swappedLintTables(path, doc)
}

// swappedLintTables reports a size-budget table written where tolerated findings
// belong ([lint.budget.skill], [lint.tolerate.skill]) and the reverse
// ([lint.budgets] AR201 = 1).
func swappedLintTables(path string, doc map[string]any) error {
	lint, ok := doc["lint"].(map[string]any)
	if !ok {
		return nil
	}
	for _, name := range []string{"tolerate", "budget"} {
		table, _ := lint[name].(map[string]any)
		for _, key := range sortedKeys(table) {
			if _, isTable := table[key].(map[string]any); isTable {
				return oops.With("path", path).
					Hint("[lint.tolerate] maps a rule code or name to a number, for example AR201 = 1; size limits go in "+lintBudgetsPath).
					Errorf("[lint.%s.%s] is a table, but [lint.%s] only holds numbers: for size limits use [lint.budgets.%s] (max_lines, max_tokens)",
						name, key, name, key)
			}
		}
	}
	budgets, _ := lint["budgets"].(map[string]any)
	for _, key := range sortedKeys(budgets) {
		if _, isTable := budgets[key].(map[string]any); !isTable {
			return oops.With("path", path).
				Hint("[lint.budgets.<kind>] sets max_lines and max_tokens for one content kind (rule, context, skill, agent, command); tolerated findings go in "+lintToleratePath).
				Errorf("[lint.budgets] %s = %v is not a size budget: to tolerate findings of a rule use [lint.tolerate] %s = %v", key, budgets[key], key, budgets[key])
		}
	}
	return nil
}

// warnDeprecatedLintBudget tells once per config file that [lint.budget] was
// renamed; the config remembers the file when it decodes it.
func (c *Config) warnDeprecatedLintBudget() {
	if c.deprecatedLintBudgetPath == "" {
		return
	}
	c.WarnOnce("lint-budget\x00"+c.deprecatedLintBudgetPath, "[lint.budget] is deprecated: rename it to [lint.tolerate] (it still works for now; [lint.budgets.<kind>] is the separate table for size limits)", "path", c.deprecatedLintBudgetPath)
}
