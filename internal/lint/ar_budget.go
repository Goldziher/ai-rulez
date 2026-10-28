package lint

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/presets"
)

// CodeLoadBudget reports content that a harness truncates or drops when it loads
// it, so the part past the limit never reaches the model.
const CodeLoadBudget = "AR964"

func registerArBudget(s *ruleSet) {
	s.addRules(RuleInfo{CodeLoadBudget, "load-budget-exceeded", SeverityWarning, "content exceeds a documented load limit of a configured harness (Claude skill listing per skill and in total, Codex AGENTS.md chain and skill listing, Windsurf/Devin rule files, Cursor rule length)"})
	s.addRunCheck(checkLoadBudgets, AnalyzerPlugin)
}

// loadBudget is one documented limit. The table below is the single place these
// numbers live; update the figures, Source and Checked together.
type loadBudget struct {
	ID      string
	Presets []string // the limit applies when one of these presets is configured; empty means every harness
	Scope   string   // "listing" per skill, scopeChain over all rules and context, scopeFile per rule or context file, keySkills over all skills
	Unit    string   // chars, bytes or lines
	Limit   int
	Source  string
	Checked string // date the figure was last compared with Source
	// Verified is false for figures taken from a secondary summary and not yet
	// compared with the vendor page.
	Verified bool
	// Label names the harness in messages when the preset name alone is unclear.
	Label string
	// CoveredBy names the rule that already enforces the limit (nothing is emitted here).
	CoveredBy string
}

const loadBudgetsChecked = "2026-10-05"

// loadBudgetClaudeListingTotal is the row of the whole Claude skill listing. Its
// limit scales with the context window, so the run takes it from the exposure
// measure (WithExposure) and the row's Limit is the figure at 200000 tokens.
const loadBudgetClaudeListingTotal = "claude-skill-listing-total"

var loadBudgets = []loadBudget{
	{ID: "claude-skill-listing", Presets: []string{presetClaude}, Scope: "listing", Unit: unitChars, Limit: 1536, Source: "https://code.claude.com/docs/en/skills", Checked: loadBudgetsChecked, Verified: true},
	{ID: loadBudgetClaudeListingTotal, Presets: []string{presetClaude}, Scope: keySkills, Unit: unitChars, Limit: 8000, Source: "https://code.claude.com/docs/en/skills", Checked: "2026-10-28", Verified: true},
	{ID: "codex-agents-chain", Presets: []string{presetCodex}, Scope: scopeChain, Unit: unitBytes, Limit: 32768, Source: "https://learn.chatgpt.com/docs/agent-configuration/agents-md", Checked: loadBudgetsChecked, Verified: true},
	{ID: "codex-skill-listing", Presets: []string{presetCodex}, Scope: keySkills, Unit: unitChars, Limit: 8000, Source: "https://learn.chatgpt.com/docs/agent-configuration/skills", Checked: loadBudgetsChecked},
	{ID: "windsurf-rule-file", Label: "Windsurf (devin preset)", Presets: []string{"devin"}, Scope: scopeFile, Unit: unitChars, Limit: 12000, Source: "https://docs.devin.ai/desktop/cascade/memories", Checked: loadBudgetsChecked, Verified: true},
	{ID: "cursor-rule-lines", Presets: []string{presetCursor}, Scope: scopeFile, Unit: unitLines, Limit: 500, Source: "https://cursor.com/docs/context/rules", Checked: loadBudgetsChecked, Verified: true},
	{ID: "agent-skills-body", Scope: scopeFile, Unit: unitLines, Limit: 500, Source: "https://agentskills.io/specification", Checked: loadBudgetsChecked, CoveredBy: CodeSizeLines},
	{ID: "agent-skills-tokens", Scope: scopeFile, Unit: "tokens", Limit: 5000, Source: "https://agentskills.io/specification", Checked: loadBudgetsChecked, CoveredBy: CodeSizeTokens},
}

// LoadBudgetIDs lists the ids [lint.load_budgets] may override, in table order.
func LoadBudgetIDs() []string {
	var ids []string
	for i := range loadBudgets {
		if loadBudgets[i].CoveredBy == "" {
			ids = append(ids, loadBudgets[i].ID)
		}
	}
	return ids
}

// LoadBudgetDefault is the built-in limit of a load budget id, 0 when unknown.
func LoadBudgetDefault(id string) int {
	for i := range loadBudgets {
		if loadBudgets[i].ID == id {
			return loadBudgets[i].Limit
		}
	}
	return 0
}

func (b loadBudget) applies(configured map[string]bool) bool {
	if b.CoveredBy != "" {
		return false
	}
	for _, p := range b.Presets {
		if configured[p] {
			return true
		}
	}
	return len(b.Presets) == 0
}

func measure(unit, text string) int {
	switch unit {
	case unitBytes:
		return len(text)
	case unitLines:
		return len(strings.Split(strings.TrimRight(text, "\n"), "\n"))
	}
	return utf8.RuneCountInString(text)
}

func checkLoadBudgets(r *runner) { //nolint:gocyclo // linear checks over a documented schema; splitting them hides the rules
	configured := r.presetNames()
	cfgPath := r.configFilePath()
	cfgLines := r.fileLines(cfgPath)
	for i := range loadBudgets {
		own := loadBudgets[i]
		b := &own
		if !b.applies(configured) {
			continue
		}
		if b.ID == loadBudgetClaudeListingTotal {
			checkClaudeListingTotal(r, b, cfgPath, cfgLines)
			continue
		}
		if v := r.lc.LoadBudgets[b.ID]; v > 0 {
			b.Limit = v
		}
		total, name, imported := 0, 0, 0
		var listed map[int]bool
		if b.Scope == keySkills {
			listed = r.listedSkills()
		}
		for i := range r.items {
			it := &r.items[i]
			// Sums measure what the harness loads, so included and installed items
			// count; a finding on a single file is only raised for files the user owns.
			if it.isDoc || (it.targetOnly && r.pathIgnored(it.abs)) {
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
				n := measure(b.Unit, desc)
				if n > b.Limit && !it.owned && cfgPath != "" {
					r.add(CodeLoadBudget, cfgPath, 1,
						"the %s skill from an include or installed skill has a description and when_to_use of %d %s, past the %d-%s cap of the %s skill listing (%s); the rest is cut off", itemID(it.kind, it.cf), n, b.Unit, b.Limit, b.Unit, b.presetLabel(), b.Source)
				} else if n > b.Limit && it.owned {
					r.add(CodeLoadBudget, it.abs, r.docs[it.abs].lineOf(keyDescription, 1),
						"description and when_to_use are %d %s, past the %d-%s cap of the %s skill listing (%s); the rest is cut off", n, b.Unit, b.Limit, b.Unit, b.presetLabel(), b.Source)
				}
			case scopeFile:
				if !it.owned {
					continue
				}
				if it.kind != kindRule && it.kind != kindContext {
					continue
				}
				if n := measure(b.Unit, it.cf.Content); n > b.Limit {
					r.add(CodeLoadBudget, it.abs, 1, "%s is %d %s, past the %d-%s limit of %s (%s)", it.kind, n, b.Unit, b.Limit, b.Unit, b.presetLabel(), b.Source)
				}
			case scopeChain:
				if it.kind == kindRule || it.kind == kindContext {
					total += measure(b.Unit, it.cf.Content)
				}
			case keySkills:
				if it.kind == kindSkill && listed[i] {
					total += measure(b.Unit, skillListingName(it)+r.description(it))
					name++
					if !it.owned {
						imported++
					}
				}
			}
		}
		fallback := 0
		if b.Scope == keySkills {
			var extra int
			extra, fallback = r.fallbackListing(b.Unit)
			total += extra
			name += fallback
		}
		note := fallbackNote(fallback) + importedNote(imported)
		if b.Scope != scopeChain && b.Scope != keySkills || cfgPath == "" {
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
		case total > b.Limit && b.Scope == scopeChain:
			r.add(CodeLoadBudget, cfgPath, at, "rules and context add up to %d %s, past the %d-%s AGENTS.md chain limit of %s (%s); content past it is not loaded", total, b.Unit, b.Limit, b.Unit, b.presetLabel(), b.Source)
		case total*10 >= b.Limit*9 && b.Scope == scopeChain:
			r.addSev(SeverityInfo, CodeLoadBudget, cfgPath, at, "rules and context add up to %d of %d %s, within 10%% of the AGENTS.md chain limit of %s (%s)", total, b.Limit, b.Unit, b.presetLabel(), b.Source)
		case total > b.Limit && b.Scope == keySkills:
			r.add(CodeLoadBudget, cfgPath, at, "the %d skill names and descriptions%s add up to %d %s, past the %d-%s listing budget %s gives skills (about 2%% of the context window, %s); skills past it are cut from the listing%s",
				name, note, total, b.Unit, b.Limit, b.Unit, b.presetLabel(), b.Source, fallbackAdvice(fallback))
		case total*10 >= b.Limit*9 && b.Scope == keySkills:
			r.addSev(SeverityInfo, CodeLoadBudget, cfgPath, at, "the %d skill names and descriptions%s add up to %d of %d %s, within 10%% of the skill listing budget of %s (%s)",
				name, fallbackNote(fallback), total, b.Limit, b.Unit, b.presetLabel(), b.Source)
		}
	}
}

// checkClaudeListingTotal reports the start-of-session skill listing of the
// native Claude Code inventory against Claude Code's listing budget, 1% of the
// context window at 4 characters per token. It runs only with an [exposure]
// table, whose inventory measures what Claude Code lists rather than what
// ai-rulez generates. The finding is on the config and depends on the
// start-of-session artifacts and the exposure inputs, so --changed keeps it
// when a listed skill grows.
func checkClaudeListingTotal(r *runner, b *loadBudget, cfgPath string, cfgLines []string) {
	start := r.exposureStart()
	if start == nil || cfgPath == "" {
		return
	}
	limit, basis := start.ListingBudgetChars, fmt.Sprintf("1%% of a %d-token context window at about 4 characters per token", start.ContextWindow)
	if v := r.lc.LoadBudgets[b.ID]; v > 0 {
		limit, basis = v, "[lint.load_budgets] "+b.ID
	}
	at := max(1, lineContaining(cfgLines, `"`+presetClaude+`"`))
	before := len(r.findings)
	switch {
	case start.ListingChars > limit:
		r.add(CodeLoadBudget, cfgPath, at, "the %d skills a session lists at start add up to %d %s, past the %d-%s skill listing budget of %s (%s, %s); Claude Code drops descriptions to fit, starting with the skills invoked least",
			start.ListingSkills, start.ListingChars, b.Unit, limit, b.Unit, b.presetLabel(), basis, b.Source)
	case start.ListingChars*10 >= limit*9:
		r.addSev(SeverityInfo, CodeLoadBudget, cfgPath, at, "the %d skills a session lists at start add up to %d of %d %s, within 10%% of the skill listing budget of %s (%s, %s)",
			start.ListingSkills, start.ListingChars, limit, b.Unit, b.presetLabel(), basis, b.Source)
	}
	r.exposureDeps(before, cfgPath, start.StartPaths)
}

// scopedFallbackSkills is the skills rules.scoped_fallback = "skill" would write
// for the project, minus the ones an authored skill or command already names;
// they count toward the skill listing of the harnesses that read .agents/skills.
// The skills are built once per run: each skill-listing budget asks for them.
func (r *runner) scopedFallbackSkills() []config.ContentFile {
	r.fallbackOnce.Do(func() {
		if r.cfg == nil || r.cfg.Content == nil || !config.SharedAgentsMDInlining(r.cfg).ScopedAsSkills {
			return
		}
		r.fallbackMemo = presets.ScopedFallbackSkillsUnclaimed(r.cfg.Content, r.cfg, r.cfg.AgentsMDOwners(), nil)
	})
	return r.fallbackMemo
}

// fallbackListing is the size, in unit, of the skill listing entries of the
// scoped-fallback skills and how many there are.
func (r *runner) fallbackListing(unit string) (size, count int) {
	skills := r.scopedFallbackSkills()
	for i := range skills {
		size += measure(unit, skills[i].ID+" "+config.SkillDescription(skills[i].Metadata))
		count++
	}
	return size, count
}

func fallbackNote(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprintf(" (%d of them scoped-fallback skills for glob-scoped rules and context)", n)
}

func importedNote(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprintf(" (%d of them from includes or installed skills)", n)
}

func fallbackAdvice(n int) string {
	if n == 0 {
		return ""
	}
	return "; set [rules] scoped_fallback_group to fold them into fewer skills, or fallback: false on an item to keep it out"
}

// listedSkills picks, by index into r.items, the one skill per name a harness
// lists. A mirror of a skill (skills/<name>/SKILL.md, a plugin bundle or its
// harness outputs) has the same name and is listed once, so the configured copy
// is preferred over an unmanaged one, then the first found.
func (r *runner) listedSkills() map[int]bool {
	chosen := map[string]int{}
	for i := range r.items {
		it := &r.items[i]
		if it.kind != kindSkill || it.isDoc || (it.targetOnly && r.pathIgnored(it.abs)) {
			continue
		}
		id := itemID(it.kind, it.cf)
		if j, ok := chosen[id]; !ok || r.items[j].targetOnly && !it.targetOnly {
			chosen[id] = i
		}
	}
	listed := make(map[int]bool, len(chosen))
	for _, i := range chosen {
		listed[i] = true
	}
	return listed
}

func skillListingName(it *item) string { return itemID(it.kind, it.cf) + " " }

func (b loadBudget) presetLabel() string {
	label := "every harness"
	switch {
	case b.Label != "":
		label = b.Label
	case len(b.Presets) > 0:
		label = fmt.Sprintf("the %s harness", strings.Join(b.Presets, "/"))
	}
	if !b.Verified && b.CoveredBy == "" {
		label += " (unverified figure)"
	}
	return label
}
