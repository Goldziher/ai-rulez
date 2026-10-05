package lint

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// CodeLoadBudget reports content that a harness truncates or drops when it loads
// it, so the part past the limit never reaches the model.
const CodeLoadBudget = "AR964"

func init() {
	registerRules(RuleInfo{CodeLoadBudget, "load-budget-exceeded", SeverityWarning, "content exceeds a documented load limit of a configured harness (Claude skill listing, Codex AGENTS.md chain and skill listing, Windsurf/Devin rule files, Cursor rule length)"})
	registerRunCheck(checkLoadBudgets)
}

// loadBudget is one documented limit. The table below is the single place these
// numbers live; update the figures, Source and Checked together.
type loadBudget struct {
	ID      string
	Presets []string // the limit applies when one of these presets is configured; empty means every harness
	Scope   string   // "listing" per skill, "chain" over all rules and context, "file" per rule or context file, "skills" over all skills
	Unit    string   // chars, bytes or lines
	Limit   int
	Source  string
	Checked string // date the figure was last compared with Source
	// Verified is false for figures taken from a secondary summary and not yet
	// compared with the vendor page.
	Verified bool
	// CoveredBy names the rule that already enforces the limit (nothing is emitted here).
	CoveredBy string
}

const loadBudgetsChecked = "2026-10-05"

var loadBudgets = []loadBudget{
	{ID: "claude-skill-listing", Presets: []string{"claude"}, Scope: "listing", Unit: "chars", Limit: 1536, Source: "https://code.claude.com/docs/en/skills", Checked: loadBudgetsChecked, Verified: true},
	{ID: "codex-agents-chain", Presets: []string{"codex"}, Scope: "chain", Unit: "bytes", Limit: 32768, Source: "https://learn.chatgpt.com/docs/agent-configuration/agents-md", Checked: loadBudgetsChecked, Verified: true},
	{ID: "codex-skill-listing", Presets: []string{"codex"}, Scope: "skills", Unit: "chars", Limit: 8000, Source: "https://learn.chatgpt.com/docs/agent-configuration/skills", Checked: loadBudgetsChecked},
	{ID: "windsurf-rule-file", Presets: []string{"devin"}, Scope: "file", Unit: "chars", Limit: 12000, Source: "https://docs.devin.ai/desktop/cascade/memories", Checked: loadBudgetsChecked, Verified: true},
	{ID: "cursor-rule-lines", Presets: []string{"cursor"}, Scope: "file", Unit: "lines", Limit: 500, Source: "https://cursor.com/docs/context/rules", Checked: loadBudgetsChecked, Verified: true},
	{ID: "agent-skills-body", Scope: "file", Unit: "lines", Limit: 500, Source: "https://agentskills.io/specification", Checked: loadBudgetsChecked, CoveredBy: CodeSizeLines},
	{ID: "agent-skills-tokens", Scope: "file", Unit: "tokens", Limit: 5000, Source: "https://agentskills.io/specification", Checked: loadBudgetsChecked, CoveredBy: CodeSizeTokens},
}

func (b loadBudget) applies(presets map[string]bool) bool {
	if b.CoveredBy != "" {
		return false
	}
	for _, p := range b.Presets {
		if presets[p] {
			return true
		}
	}
	return len(b.Presets) == 0
}

func measure(unit, text string) int {
	switch unit {
	case "bytes":
		return len(text)
	case "lines":
		return len(strings.Split(strings.TrimRight(text, "\n"), "\n"))
	}
	return utf8.RuneCountInString(text)
}

func checkLoadBudgets(r *runner) {
	presets := r.presetNames()
	cfgPath := r.configFilePath()
	cfgLines := r.fileLines(cfgPath)
	for _, b := range loadBudgets {
		if !b.applies(presets) {
			continue
		}
		total, name := 0, 0
		for i := range r.items {
			it := &r.items[i]
			if !it.owned || it.isDoc {
				continue
			}
			switch b.Scope {
			case "listing":
				if it.kind != kindSkill {
					continue
				}
				desc := r.description(it)
				if it.cf.Metadata != nil {
					desc += it.cf.Metadata.Extra["when_to_use"]
				}
				if n := measure(b.Unit, desc); n > b.Limit {
					r.add(CodeLoadBudget, it.abs, r.docs[it.abs].lineOf("description", 1),
						"description and when_to_use are %d %s, past the %d-%s cap of the %s skill listing (%s); the rest is cut off", n, b.Unit, b.Limit, b.Unit, b.presetLabel(), b.Source)
				}
			case "file":
				if it.kind != kindRule && it.kind != kindContext {
					continue
				}
				if n := measure(b.Unit, it.cf.Content); n > b.Limit {
					r.add(CodeLoadBudget, it.abs, 1, "%s is %d %s, past the %d-%s limit of %s (%s)", it.kind, n, b.Unit, b.Limit, b.Unit, b.presetLabel(), b.Source)
				}
			case "chain":
				if it.kind == kindRule || it.kind == kindContext {
					total += measure(b.Unit, it.cf.Content)
				}
			case "skills":
				if it.kind == kindSkill {
					total += measure(b.Unit, skillListingName(it)+r.description(it))
					name++
				}
			}
		}
		if b.Scope != "chain" && b.Scope != "skills" || cfgPath == "" {
			continue
		}
		at := 1
		for _, p := range b.Presets {
			if l := lineContaining(cfgLines, `"`+p+`"`); l > 1 {
				at = l
				break
			}
		}
		switch {
		case total > b.Limit && b.Scope == "chain":
			r.add(CodeLoadBudget, cfgPath, at, "rules and context add up to %d %s, past the %d-%s AGENTS.md chain limit of %s (%s); content past it is not loaded", total, b.Unit, b.Limit, b.Unit, b.presetLabel(), b.Source)
		case total*10 >= b.Limit*9 && b.Scope == "chain":
			r.addSev(SeverityInfo, CodeLoadBudget, cfgPath, at, "rules and context add up to %d of %d %s, within 10%% of the AGENTS.md chain limit of %s (%s)", total, b.Limit, b.Unit, b.presetLabel(), b.Source)
		case total > b.Limit && b.Scope == "skills":
			r.addSev(SeverityInfo, CodeLoadBudget, cfgPath, at, "the %d skill names and descriptions add up to %d %s; %s lists skills within about 2%% of the context window (%d %s when the window is unknown, %s)", name, total, b.Unit, b.presetLabel(), b.Limit, b.Unit, b.Source)
		}
	}
}

func skillListingName(it *item) string { return itemID(it.kind, it.cf) + " " }

func (b loadBudget) presetLabel() string {
	if len(b.Presets) == 0 {
		return "every harness"
	}
	return fmt.Sprintf("the %s harness", strings.Join(b.Presets, "/"))
}
