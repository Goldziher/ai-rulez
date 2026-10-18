package lint

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/harnessnames"
)

// Codes of the harness built-in name checks.
const (
	// CodeBuiltinNameCollision reports an output named like a built-in of a harness that loads it.
	CodeBuiltinNameCollision = "AR9U0"
	// CodeReservedHarnessName reports an output named like a name the harness refuses to load.
	CodeReservedHarnessName = "AR9U1"

	maxProjectNameFiles    = 64
	maxProjectNameFileSize = 256 << 10
)

// ReachTarget is one harness that loads an output of the project: the preset, the
// role it reads the output in (skill, command or agent) and the directory it reads it from.
type ReachTarget struct {
	Preset, Role, Dir string
	// Writer says the preset itself writes the output there; a reader only reads a
	// directory another preset writes (.agents/skills).
	Writer bool
}

// HarnessReach tells which harnesses load an output of the project. The generator
// implements it (it knows the layouts of the presets and the plan); lint cannot import
// the generator.
type HarnessReach interface {
	// Reach returns the harnesses that load the project item of the kind (skill, command or
	// agent) with the output id: every preset that writes it and every harness that reads the
	// directory it lands in. An item the plan does not write (plugin-only) reaches none.
	Reach(kind, id string) []ReachTarget
}

// WithHarnessReach hands the checks the harnesses that load each output. Without it a
// skill, command or agent is judged against the configured presets only.
func WithHarnessReach(h HarnessReach) Option { return func(r *runner) { r.reach = h } }

func registerArBuiltinNames(s *ruleSet) {
	s.addRules(
		RuleInfo{CodeBuiltinNameCollision, "harness-builtin-name-collision", SeverityError, "a skill, command or agent is named like a built-in command, bundled skill or built-in agent of a harness that loads it (allow an intended collision with lint.builtin_names.allow; an outcome the harness does not document is only informational)"},
		RuleInfo{CodeReservedHarnessName, "harness-reserved-name", SeverityError, "a skill, command or agent is named like a name the harness refuses to load; lint.builtin_names.allow does not apply"},
	)
	s.addDocs(map[string]RuleDoc{
		CodeBuiltinNameCollision: {
			Why:  "A harness has built-in commands, bundled skills and agents, and what happens to a user item of the same name differs: the user item replaces the built-in (Claude Code, OpenCode, Qwen Code), the built-in stays and the user item is renamed or unreachable (Gemini CLI, Cline, Goose, Augment), or both are listed (Codex). The tables are data in `internal/harnessnames/data/<preset>.toml`, one file per preset, each group with the page it was taken from and, where the harness documents the outcome, a verbatim quote. The rule checks every output against every preset that writes it and every harness that reads the directory it lands in (`.agents/skills` is read by many harnesses whether or not their preset is enabled). Names are compared as the harness derives them: surrounding space trimmed, NFKC and case-folded; the directory or file name and the `name:` frontmatter are both checked; aliases count. Outputs a harness always invokes under a prefix (plugin-only content) are exempt. The severity follows the table: error for a preset the project writes for whose table records a quoted outcome and is set to error, warning for a harness that only reads a shared directory such as `.agents/skills` without its preset being enabled, for an unquoted outcome, a `both_listed` outcome and names you declare yourself, and information for a built-in whose collision outcome the vendor does not document (listed once per item, not an error). A table can be incomplete: `completeness` is `partial` or `docs-only` for most harnesses, because vendor docs list a floor, not a ceiling. Claude Code skills and commands are reported by AR808, not here. Acknowledge an intended collision with `lint.builtin_names.allow = [\"name\", \"preset:name\"]` (`lint.allow_overrides` is honored the same way), add names the table cannot know with `[lint.builtin_names.extra]`, or add a table in `.ai-rulez/harness-names/<preset>.toml`. `[lint.severity] AR9U0 = \"warning\"` downgrades the rule everywhere, `[lint.builtin_names] enabled = false` turns it off.",
			Bad:  "A skill `help` while the gemini preset is configured (Gemini CLI keeps its `/help`; the skill becomes `help1`)",
			Good: "Rename the skill (`project-help`), or list `gemini:help` in lint.builtin_names.allow",
		},
		CodeReservedHarnessName: {
			Why:  "A harness refuses to load some names: Claude Code skips a skill or command named `synced` or `anthropic-skills`, or starting with `anthropic-skills:`, outside a plugin. Unlike a built-in, there is nothing to acknowledge: the item never loads, so `lint.builtin_names.allow` and `lint.allow_overrides` do not silence it. Rename the item.",
			Bad:  "`.ai-rulez/skills/synced/SKILL.md` with the claude preset",
			Good: "Rename the skill (`sync-notes`)",
		},
	})
	s.addItemCheck(checkHarnessBuiltinName, AnalyzerTraps)
}

// nameTables caches the merged tables of a run.
type nameTables struct {
	project map[string]*harnessnames.Table
	merged  map[string]*harnessnames.Table
}

// projectNameTables reads .ai-rulez/harness-names/*.toml once. A file that cannot be used is
// reported (as a warning of AR9U0) and skipped.
func (r *runner) projectNameTables() map[string]*harnessnames.Table {
	r.namesOnce.Do(func() {
		r.harnessNames.project = map[string]*harnessnames.Table{}
		r.harnessNames.merged = map[string]*harnessnames.Table{}
		if r.cfg == nil || r.cfg.ConfigDir == "" || r.cfg.Contentless() {
			return
		}
		files, err := filepath.Glob(filepath.Join(r.cfg.ConfigDir, "harness-names", "*.toml"))
		if err != nil || len(files) == 0 {
			return
		}
		sort.Strings(files)
		for i, file := range files {
			abs, _ := filepath.Abs(file) //nolint:errcheck // display only
			if i >= maxProjectNameFiles {
				r.addSev(SeverityWarning, CodeBuiltinNameCollision, abs, 1, "more than %d harness name files; this one is not read", maxProjectNameFiles)
				continue
			}
			preset := strings.TrimSuffix(filepath.Base(file), ".toml")
			t, problem := readProjectNames(file, preset)
			if problem != "" {
				r.addSev(SeverityWarning, CodeBuiltinNameCollision, abs, 1, "%s; the file is not used", problem)
				continue
			}
			r.harnessNames.project[preset] = t
		}
	})
	return r.harnessNames.project
}

func readProjectNames(file, preset string) (table *harnessnames.Table, problem string) {
	info, err := os.Stat(file)
	if err != nil || !info.Mode().IsRegular() {
		return nil, "harness names file is not a regular file"
	}
	if info.Size() > maxProjectNameFileSize {
		return nil, fmt.Sprintf("harness names file is over %d bytes", maxProjectNameFileSize)
	}
	data, err := os.ReadFile(file) //nolint:gosec // a project file under the config directory
	if err != nil {
		return nil, "read harness names file: " + err.Error()
	}
	t, err := harnessnames.ParseProject(data, preset)
	if err != nil {
		return nil, "invalid harness names file: " + err.Error()
	}
	return t, ""
}

// namesFor returns the table of a preset with the project additions and the declared extra
// names merged in, or nil when the preset has none.
func (r *runner) namesFor(preset string) *harnessnames.Table {
	project := r.projectNameTables()
	r.namesMu.Lock()
	defer r.namesMu.Unlock()
	if t, ok := r.harnessNames.merged[preset]; ok {
		return t
	}
	t, _ := harnessnames.For(preset)
	t = t.WithProject(project[preset])
	if bn := r.lc.BuiltinNames; bn != nil {
		for key, names := range bn.Extra {
			if strings.EqualFold(strings.TrimSpace(key), preset) {
				t = t.WithProject(harnessnames.Declared(preset, names))
			}
		}
	}
	r.harnessNames.merged[preset] = t
	return t
}

// reachOf returns the harnesses that load the output of an item.
func (r *runner) reachOf(kind, id string) []ReachTarget {
	if r.reach != nil {
		return r.reach.Reach(kind, id)
	}
	names := r.presetNames()
	if len(names) == 0 {
		names = map[string]bool{presetClaude: true}
	}
	var out []ReachTarget
	for p := range names {
		out = append(out, ReachTarget{Preset: strings.ToLower(p), Role: kind, Writer: true})
	}
	slices.SortFunc(out, func(a, b ReachTarget) int { return strings.Compare(a.Preset, b.Preset) })
	return out
}

// builtinAllowed reports whether lint.builtin_names.allow or lint.allow_overrides lists the
// collision as "name" or "preset:name".
func (r *runner) builtinAllowed(preset, name string) bool {
	lists := [][]string{r.lc.AllowOverrides}
	if r.lc.BuiltinNames != nil {
		lists = append(lists, r.lc.BuiltinNames.Allow)
	}
	want := harnessnames.Normalize(name)
	for _, list := range lists {
		for _, entry := range list {
			scope, n := splitScoped(entry)
			if harnessnames.Normalize(n) == want && (scope == "" || scope == preset) {
				return true
			}
		}
	}
	return false
}

// splitScoped splits "preset:name"; a prefix that is not a known preset is part of the name.
func splitScoped(entry string) (scope, name string) {
	if i := strings.Index(entry, ":"); i > 0 {
		prefix := strings.ToLower(strings.TrimSpace(entry[:i]))
		if _, ok := harnessnames.For(prefix); ok {
			return prefix, entry[i+1:]
		}
	}
	return "", entry
}

// nameCandidate is a name an item can be known by.
type nameCandidate struct {
	name string
	line int
}

func checkHarnessBuiltinName(r *runner, it *item, d doc, _ frontmatter) {
	if !it.owned || it.targetOnly || !r.lc.BuiltinNames.BuiltinNamesEnabled() {
		return
	}
	if it.kind != kindSkill && it.kind != kindCommand && it.kind != kindAgent {
		return
	}
	id := itemID(it.kind, it.cf)
	cands := nameCandidates(it, d, id)
	targets := r.reachOf(it.kind, id)
	seen := map[string]bool{}
	var unknown []string
	for _, c := range cands {
		for _, t := range targets {
			key := t.Preset + "\x00" + harnessnames.Normalize(c.name)
			if seen[key] {
				continue
			}
			hits := r.namesFor(t.Preset).Find(t.Role, c.name)
			if len(hits) == 0 {
				continue
			}
			seen[key] = true
			if note := r.reportNameHit(it, c, t, hits[0]); note != "" {
				unknown = append(unknown, note)
			}
		}
		if len(unknown) > 0 {
			r.addSev(SeverityInfo, CodeBuiltinNameCollision, it.abs, c.line,
				"%s %q has the name of a built-in of %s, whose harness does not document what happens to a user item of that name (the tables may be incomplete)",
				it.kind, c.name, strings.Join(unknown, ", "))
			unknown = nil
		}
	}
}

// nameCandidates are the names an item can be known by: its output id and its name: frontmatter.
func nameCandidates(it *item, d doc, id string) []nameCandidate {
	cands := []nameCandidate{{id, 1}}
	if it.cf.Metadata != nil {
		if n := strings.TrimSpace(it.cf.Metadata.Extra["name"]); n != "" && n != id {
			cands = append(cands, nameCandidate{n, d.lineOf("name", 1)})
		}
	}
	return cands
}

// reportNameHit reports one hit and returns the preset to list in the aggregated informational
// finding when the collision outcome is not documented.
func (r *runner) reportNameHit(it *item, c nameCandidate, t ReachTarget, h harnessnames.Hit) string {
	if h.Collision == harnessnames.NotApplied {
		return ""
	}
	if h.Kind == harnessnames.KindReserved || h.Collision == harnessnames.Error {
		r.addSev(SeverityError, CodeReservedHarnessName, it.abs, c.line,
			"%s %q is a name the %s harness refuses to load (%s); rename it, lint.builtin_names.allow does not apply",
			it.kind, c.name, h.Preset, evidenceLine(h))
		return ""
	}
	if !h.Declared && h.Preset == presetClaude && (h.Kind == harnessnames.KindCommand || h.Kind == harnessnames.KindSkill) &&
		slices.Contains(r.reservedPresets(), presetClaude) {
		return "" // AR808 reports the Claude Code commands and bundled skills
	}
	if r.builtinAllowed(h.Preset, c.name) {
		return ""
	}
	if h.Collision == harnessnames.Unknown && !h.Declared {
		return h.Preset
	}
	r.addSev(nameSeverity(h, t), CodeBuiltinNameCollision, it.abs, c.line, "%s", collisionMessage(it.kind, c.name, t, h))
	return ""
}

// nameSeverity is the severity of a collision finding: the table's severity only where the
// outcome carries a verbatim quote, and never an error for a name the project declared without
// saying so, for an outcome of both being listed, or for a harness that only reads a shared
// directory (.agents/skills) and whose preset the project did not enable.
func nameSeverity(h harnessnames.Hit, t ReachTarget) Severity {
	switch {
	case h.Declared:
		if h.Severity == harnessnames.SeverityError {
			return SeverityError
		}
	case !t.Writer:
	case h.Quoted && h.Severity == harnessnames.SeverityError && h.Collision != harnessnames.BothListed:
		return SeverityError
	}
	return SeverityWarning
}

// shortVersion keeps the version of a harness_version text such as "v0.64.0 (latest stable tag)".
func shortVersion(v string) string {
	if i := strings.IndexAny(v, " ;,("); i > 0 {
		return v[:i]
	}
	return v
}

func evidenceLine(h harnessnames.Hit) string {
	parts := []string{h.Preset}
	if h.Version != "" && !h.Declared {
		parts[0] += " " + shortVersion(h.Version)
	}
	if !h.Declared {
		parts = append(parts, "verified "+h.VerifiedOn)
	} else {
		parts = append(parts, "declared by the project")
	}
	if h.Evidence != nil {
		parts = append(parts, h.Evidence.URL)
	}
	return strings.Join(parts, ", ")
}

func collisionMessage(kind, name string, t ReachTarget, h harnessnames.Hit) string {
	what := "built-in command /" + h.Name
	switch h.Kind {
	case harnessnames.KindSkill:
		what = "bundled skill " + h.Name
	case harnessnames.KindAgent:
		what = "built-in agent " + h.Name
	}
	if h.Alias {
		what = "alias /" + h.Name + " of /" + h.AliasOf
	}
	var consequence string
	switch h.Collision {
	case harnessnames.UserWins:
		consequence = fmt.Sprintf("Your %s replaces the built-in.", kind)
	case harnessnames.BuiltinWins:
		consequence = fmt.Sprintf("The harness keeps the built-in and renames or ignores your %s.", kind)
	case harnessnames.BothListed:
		consequence = "Both are listed, and the harness can pick the built-in."
	default:
		consequence = "The outcome of the collision is not documented."
	}
	where := ""
	if !t.Writer && t.Dir != "" {
		where = fmt.Sprintf(" The harness reads %s.", t.Dir)
	}
	return fmt.Sprintf("%s %q collides with the %s %s (%s).%s %s Rename it, or add %q to lint.builtin_names.allow.",
		kind, name, h.Preset, what, evidenceLine(h), where, consequence, h.Preset+":"+harnessnames.Normalize(name))
}

// namesPresetInUse reports whether the project writes for the harness (the default, claude,
// when no preset is configured), so a stale name table matters.
func (r *runner) namesPresetInUse(preset string) bool {
	names := r.presetNames()
	if len(names) == 0 {
		return preset == presetClaude
	}
	for n := range names {
		if strings.EqualFold(n, preset) {
			return true
		}
	}
	return false
}

// validateBuiltinNames reports lint.builtin_names settings that cannot work.
func validateBuiltinNames(bn *config.LintBuiltinNames) []string {
	if bn == nil {
		return nil
	}
	var problems []string
	for preset, names := range bn.Extra {
		if _, ok := harnessnames.For(strings.ToLower(strings.TrimSpace(preset))); !ok {
			problems = append(problems, fmt.Sprintf("lint.builtin_names.extra: unknown preset %q (known: %s)", preset, strings.Join(harnessnames.Presets(), ", ")))
		}
		for _, n := range names {
			if strings.TrimSpace(n) == "" || strings.ContainsAny(n, "/\\") {
				problems = append(problems, fmt.Sprintf("lint.builtin_names.extra.%s: %q is not a name", preset, n))
			}
		}
	}
	for _, a := range bn.Allow {
		if _, n := splitScoped(a); strings.TrimSpace(n) == "" {
			problems = append(problems, fmt.Sprintf("lint.builtin_names.allow: %q names nothing", a))
		}
	}
	return problems
}
