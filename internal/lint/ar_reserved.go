package lint

import (
	"sort"
	"strings"
	"sync"

	"github.com/Goldziher/ai-rulez/v5/internal/harnessnames"
)

// CodeReservedName reports a skill or command named like a harness built-in.
const CodeReservedName = "AR808"

// reservedNames returns the reserved names per preset, normalized, built once.
var reservedNames = sync.OnceValue(func() map[string]map[string]string {
	table := reservedPresetTable()
	out := make(map[string]map[string]string, len(table))
	for preset, p := range table {
		set := make(map[string]string, len(p.Names))
		for _, n := range p.Names {
			set[normalizeReservedName(n)] = p.Title
		}
		out[preset] = set
	}
	return out
})

// reservedAliases returns, per preset, the aliases of the built-ins keyed by normalized
// name, mapped to the command they belong to.
var reservedAliases = sync.OnceValue(func() map[string]map[string]string {
	table := reservedPresetTable()
	out := make(map[string]map[string]string, len(table))
	for preset, p := range table {
		set := make(map[string]string, len(p.Aliases))
		for alias, of := range p.Aliases {
			set[normalizeReservedName(alias)] = of
		}
		out[preset] = set
	}
	return out
})

// normalizeReservedName folds a name the way an assistant matches a slash
// command: NFKC and case-insensitive, surrounding space trimmed, underscores and
// inner spaces read as hyphens.
func normalizeReservedName(n string) string {
	return strings.NewReplacer("_", "-", " ", "-").Replace(harnessnames.Normalize(n))
}

func registerArReserved(s *ruleSet) {
	s.addRules(RuleInfo{CodeReservedName, "reserved-name", SeverityError, "a skill or command is named like a built-in command or bundled skill of a target assistant, so it replaces the built-in (allow an intentional override with lint.allow_overrides; only skills the harness loads are checked, see lint.skill_roots)"})
	s.addDocs(map[string]RuleDoc{
		CodeReservedName: {
			Why:  "In Claude Code a project skill or command replaces the built-in of the same name, so `.claude/skills/init` silently disables `/init`. The names are data in `internal/harnessnames/data/claude.toml` (AR808 reports the Claude Code commands and bundled skills, AR9U0 every other harness, so no name is reported twice). A name is compared NFKC-normalized and case-insensitively with surrounding space trimmed and `_` or spaces read as `-`. An alias of a built-in (`/new` for `/clear`) is reserved too: a skill replaces the built-in command but not its aliases, so the alias keeps running the built-in; the skill itself loads, so an alias is a warning and a shadowed built-in is an error. The `name:` frontmatter and the directory or file name are both checked. Acknowledge an intentional override with `lint.allow_overrides = [\"init\"]` or `[\"claude:init\"]`. A hand-written SKILL.md is checked only where the harness loads it from: `.claude/skills` (`.claude/commands` for a command), a plugin's `skills/` or `commands/` directory next to its manifest, or a local `[[skill_sources]]` directory; content under `.ai-rulez/` is always checked, except an item `validate` can see the plan does not write where the harness reads (plugin-only content, invoked under a plugin prefix). A SKILL.md elsewhere (an application's own `src/public/skills/`) is still linted for format. `[lint] skill_roots = [globs]` replaces those roots (globs match the directory that holds the skill directories, relative to the project root), `[lint] ignore_paths` drops every finding for a source file, and `validate --exclude <glob>` leaves files out of a config-free run.",
			Bad:  "`.claude/skills/init/SKILL.md` with the claude preset",
			Good: "Rename the skill (`project-init`), or list `claude:init` in lint.allow_overrides",
		},
	})
	s.addItemCheck(checkReservedName, AnalyzerDescriptions)
}

// reservedPresets lists the configured presets that have a reserved-name list;
// no configured preset means the default, claude.
func (r *runner) reservedPresets() []string {
	table := reservedNames()
	names := r.presetNames()
	var out []string
	if len(names) == 0 {
		names = map[string]bool{presetClaude: true}
	}
	for n := range names {
		if _, ok := table[strings.ToLower(n)]; ok {
			out = append(out, strings.ToLower(n))
		}
	}
	sort.Strings(out)
	return out
}

func (r *runner) reservedAllowed(preset, name string) bool {
	for _, a := range r.lc.AllowOverrides {
		a = normalizeReservedName(a)
		if a == name || a == preset+":"+name {
			return true
		}
	}
	return false
}

func checkReservedName(r *runner, it *item, d doc, _ frontmatter) {
	if (it.kind != kindSkill && it.kind != kindCommand) || !it.owned {
		return
	}
	table := reservedNames()
	candidates := []string{itemID(it.kind, it.cf)}
	if it.cf.Metadata != nil {
		if n := it.cf.Metadata.Extra["name"]; strings.TrimSpace(n) != "" {
			candidates = append(candidates, n)
		}
	}
	seen := map[string]bool{}
	for _, preset := range r.reservedPresets() {
		if !r.inHarnessRoot(it, preset) || !r.harnessLoads(it, preset) {
			continue
		}
		for _, c := range candidates {
			name := normalizeReservedName(c)
			title, ok := table[preset][name]
			if !ok || seen[preset+":"+name] || r.reservedAllowed(preset, name) {
				continue
			}
			seen[preset+":"+name] = true
			line := 1
			if strings.TrimSpace(c) != itemID(it.kind, it.cf) {
				line = d.lineOf("name", 1)
			}
			if of, isAlias := reservedAliases()[preset][name]; isAlias {
				// The docs say the skill loads and only the alias keeps running the built-in: a warning.
				r.addSev(SeverityWarning, CodeReservedName, it.abs, line, "%s %q is an alias of the %s built-in /%s (preset %s): a skill replaces the built-in command but not its aliases, so /%s keeps running the built-in; rename it, or list %q in lint.allow_overrides if that is intended",
					it.kind, strings.TrimSpace(c), title, of, preset, name, preset+":"+name)
				continue
			}
			r.add(CodeReservedName, it.abs, line, "%s %q shadows the %s built-in /%s (preset %s); rename it, or list %q in lint.allow_overrides if the override is intended",
				it.kind, strings.TrimSpace(c), title, name, preset, preset+":"+name)
		}
	}
}

// harnessLoads reports whether the harness of preset loads the output of an authored item. Without a
// reach (a run that did not render the plan) every authored item counts. With one, an item the
// plan does not write where the harness reads (plugin-only content, which a harness invokes under a
// plugin prefix, or an item whose targets leave the harness out) cannot shadow a built-in.
func (r *runner) harnessLoads(it *item, preset string) bool {
	if r.reach == nil || it.targetOnly {
		return true
	}
	for _, t := range r.reach.Reach(it.kind, itemID(it.kind, it.cf)) {
		if t.Preset == preset {
			return true
		}
	}
	return false
}
