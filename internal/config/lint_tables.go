package config

import (
	"github.com/pelletier/go-toml/v2"
	"github.com/samber/oops"
)

// Two lint tables look alike and mean different things: [lint.ratchet] maps a
// rule to a tolerated finding count, [lint.budgets.<kind>] sets the size limits
// of a content kind. The checks below turn the obvious mix-ups into one
// sentence naming the right table.

const (
	lintRatchetPath = "[lint.ratchet]"
	lintBudgetsPath = "[lint.budgets.<kind>]"
)

// swappedLintTablesTOML explains a TOML document that fails to decode because a
// lint table holds the other table's shape. It returns nil when the document is
// not that kind of mistake.
func swappedLintTablesTOML(path string, data []byte) error {
	var doc map[string]any
	if toml.Unmarshal(data, &doc) == nil {
		return swappedLintTables(path, doc)
	}
	return nil // not TOML at all: the caller's decode error stands
}

// swappedLintTables reports a size-budget table written where ratcheted findings
// belong ([lint.budget.skill], [lint.ratchet.skill]) and the reverse
// ([lint.budgets] AR201 = 1).
func swappedLintTables(path string, doc map[string]any) error {
	lint, ok := doc["lint"].(map[string]any)
	if !ok {
		return nil
	}
	for _, name := range []string{"ratchet", "tolerate", "budget"} {
		table, ok := lint[name].(map[string]any)
		if !ok {
			continue
		}
		for _, key := range sortedKeys(table) {
			if _, isTable := table[key].(map[string]any); isTable {
				return oops.With("path", path).
					Hint("[lint.ratchet] maps a rule code or name to a number, for example AR201 = 1; size limits go in "+lintBudgetsPath).
					Errorf("[lint.%s.%s] is a table, but [lint.%s] only holds numbers: for size limits use [lint.budgets.%s] (max_lines, max_tokens)",
						name, key, name, key)
			}
		}
	}
	budgets, ok := lint["budgets"].(map[string]any)
	if !ok {
		return nil
	}
	for _, key := range sortedKeys(budgets) {
		if _, isTable := budgets[key].(map[string]any); !isTable {
			return oops.With("path", path).
				Hint("[lint.budgets.<kind>] sets max_lines and max_tokens for one content kind (rule, context, skill, agent, command); ratcheted findings go in "+lintRatchetPath).
				Errorf("[lint.budgets] %s = %v is not a size budget: to ratchet findings of a rule use [lint.ratchet] %s = %v", key, budgets[key], key, budgets[key])
		}
	}
	return nil
}

// renamedRatchetTable refuses the pre-v5 spellings of [lint.ratchet]. They are
// not read, so a silent fallback would drop the counts and fail a gated build.
func renamedRatchetTable(path string, lc *LintConfig) error {
	if lc == nil {
		return nil
	}
	for _, old := range []struct {
		name   string
		counts map[string]int
	}{{"budget", lc.Budget}, {"tolerate", lc.Tolerate}} {
		if len(old.counts) > 0 {
			return oops.With("path", path).
				Hint("Run `"+MigrateCommandHint+"`; size limits stay in "+lintBudgetsPath).
				Errorf("[lint.%s] was renamed to %s in v5", old.name, lintRatchetPath)
		}
	}
	return nil
}
