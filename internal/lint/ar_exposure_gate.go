package lint

import (
	"slices"
	"strconv"
	"strings"

	fmsplit "github.com/Goldziher/ai-rulez/v5/internal/frontmatter"
)

// CodeExposureGateUnreadable is a Claude Code skill, command or rule whose
// frontmatter does not parse while it names a gate key (AR9W4).
const CodeExposureGateUnreadable = "AR9W4"

func registerArExposureGate(s *ruleSet) {
	s.addRules(
		RuleInfo{CodeExposureGateUnreadable, "exposure-gate-unreadable", SeverityWarning, "a skill, command or rule Claude Code loads has frontmatter that does not parse (AR306) while it names paths, disable-model-invocation or user-invocable, so whether it is gated, and when it loads, cannot be known"},
	)
	s.addDocs(map[string]RuleDoc{
		CodeExposureGateUnreadable: {
			Why:  "`paths:` defers a skill or rule until a matching file is read and `disable-model-invocation` keeps a skill out of the model's listing. When the block around them is not valid YAML, Claude Code (2.1.312, measured) recovers some blocks and keeps the gate (an unquoted `: ` in a value), and drops others whole (an unclosed quote, a tab indent), so the item then loads at session start in every session. Which one happens is up to the harness's own parser, so `ai-rulez exposure inventory` reports the phase as `unknown` and counts it at session start, and the MCP server never returns such a rule for a `path` query. Only for the claude preset, or a run without a configuration; AR306 reports the block itself.",
			Bad:  "`description: Use when: deploying` above `paths: [\"services/billing/**\"]`",
			Good: "`description: \"Use when: deploying\"` above `paths: [\"services/billing/**\"]`",
		},
	})
	SetAnalyzer(CodeExposureGateUnreadable, AnalyzerExposure, ScopeItem)
	s.addItemCheck(checkExposureGateUnreadable, AnalyzerExposure)
}

// gatedKinds are the item kinds Claude Code gates by frontmatter: a command is
// a skill to it.
var gatedKinds = []string{kindSkill, kindCommand, kindRule}

// checkExposureGateUnreadable reports a lost gate (AR9W4): an item AR306
// reports whose block names a gate key.
func checkExposureGateUnreadable(r *runner, it *item, d doc, _ frontmatter) {
	if !it.cf.MalformedFrontmatter || !slices.Contains(gatedKinds, it.kind) || !r.targetsClaude() || !r.harnessLoads(it, presetClaude) {
		return
	}
	keys, line := gateKeysIn(d)
	if len(keys) == 0 {
		return
	}
	quoted := make([]string, len(keys))
	for i, k := range keys {
		quoted[i] = strconv.Quote(k)
	}
	r.add(CodeExposureGateUnreadable, it.abs, line,
		"%s %q names %s in frontmatter that does not parse, so whether Claude Code keeps the gate cannot be known: it may load at session start; quote the value YAML cannot read (AR306)",
		it.kind, itemID(it.kind, it.cf), strings.Join(quoted, ", "))
}

// gateKeysIn lists the gate keys a frontmatter block names, in the order they
// appear, and the 1-based line of the first. A block that never closes runs to
// the end of the file, as the exposure inventory reads it.
func gateKeysIn(d doc) (keys []string, line int) {
	end := len(d.lines)
	if d.bodyStart > 0 {
		end = d.bodyStart - 1
	}
	for i := 1; i < end; i++ {
		for _, key := range fmsplit.GateKeys {
			if slices.Contains(keys, key) || !fmsplit.NamesKey(d.lines[i], key) {
				continue
			}
			keys = append(keys, key)
			if line == 0 {
				line = i + 1
			}
		}
	}
	return keys, line
}
