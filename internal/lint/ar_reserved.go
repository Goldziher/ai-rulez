package lint

import (
	"sort"
	"strings"
	"sync"
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

// normalizeReservedName folds a name the way an assistant matches a slash
// command: case-insensitive, surrounding space trimmed, underscores and inner
// spaces read as hyphens.
func normalizeReservedName(n string) string {
	return strings.ToLower(strings.NewReplacer("_", "-", " ", "-").Replace(strings.TrimSpace(n)))
}

func registerArReserved(s *ruleSet) {
	s.addRules(RuleInfo{CodeReservedName, "reserved-name", SeverityError, "a skill or command is named like a built-in command or bundled skill of a target assistant, so it replaces the built-in (allow an intentional override with lint.allow_overrides)"})
	s.addDocs(map[string]RuleDoc{
		CodeReservedName: {
			Why:  "In Claude Code a project skill or command replaces the built-in of the same name, so `.claude/skills/init` silently disables `/init`. The list of reserved names per preset is data in `internal/lint/ar_reserved_names.go`. A name is compared case-insensitively with surrounding space trimmed and `_` or spaces read as `-`. The `name:` frontmatter and the directory or file name are both checked. Acknowledge an intentional override with `lint.allow_overrides = [\"init\"]` or `[\"claude:init\"]`.",
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
			r.add(CodeReservedName, it.abs, line, "%s %q shadows the %s built-in /%s (preset %s); rename it, or list %q in lint.allow_overrides if the override is intended",
				it.kind, strings.TrimSpace(c), title, name, preset, preset+":"+name)
		}
	}
}
