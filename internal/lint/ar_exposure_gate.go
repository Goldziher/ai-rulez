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
		RuleInfo{CodeExposureGateUnreadable, "exposure-gate-unreadable", SeverityWarning, "a skill, command or rule Claude Code loads has frontmatter that does not parse (AR306) while it names paths, disable-model-invocation or user-invocable, or sets paths or disable-model-invocation to a value of the wrong type, so whether it is gated, and when it loads, cannot be known"},
	)
	s.addDocs(map[string]RuleDoc{
		CodeExposureGateUnreadable: {
			Why:  "`paths:` defers a skill or rule until a matching file is read and `disable-model-invocation` keeps a skill out of the model's listing. When the block around them is not valid YAML, Claude Code (2.1.312, measured) recovers some blocks and keeps the gate (an unquoted colon and space in a value), and drops others whole (an unclosed quote, a tab indent), so the item then loads at session start in every session. Which one happens is up to the harness's own parser, and a gate value of the wrong type (`paths: 5`, or `disable-model-invocation: yes`, a string to YAML 1.2 that Claude Code reads as YAML 1.1 does) leaves the same doubt, so `ai-rulez exposure inventory` reports the phase as `unknown` and counts it at session start, and the MCP server never returns such a rule for a `path` query. Only for the claude preset, or a run without a configuration; AR306 reports the block itself.",
			Bad:  "`description: Use when: deploying` above `paths: [\"services/billing/**\"]`",
			Good: "`description: \"Use when: deploying\"` above `paths: [\"services/billing/**\"]`",
		},
	})
	SetAnalyzer(CodeExposureGateUnreadable, AnalyzerExposure, ScopeItem)
	s.addItemCheck(checkExposureGateUnreadable, AnalyzerExposure)
}

// gatedKind reports an item kind Claude Code gates by frontmatter: a command is
// a skill to it.
func gatedKind(kind string) bool { return kind == kindSkill || kind == kindCommand || kind == kindRule }

// The gate keys whose values the exposure inventory reads, and must be able to.
const (
	gateKeyPaths   = "paths"
	gateKeyDisable = "disable-model-invocation"
)

// checkExposureGateUnreadable reports a lost gate (AR9W4): an item AR306
// reports whose block names a gate key, or a block that parses but holds a gate
// value of the wrong type, which the exposure inventory reads as phase unknown
// too.
func checkExposureGateUnreadable(r *runner, it *item, d doc, fm frontmatter) {
	if !gatedKind(it.kind) || !r.targetsClaude() || !r.harnessLoads(it, presetClaude) {
		return
	}
	if !it.cf.MalformedFrontmatter {
		reportWrongTypedGates(r, it, fm)
		return
	}
	keys, line := gateKeysIn(d)
	if len(keys) == 0 {
		return
	}
	r.add(CodeExposureGateUnreadable, it.abs, line,
		"%s %q names %s in frontmatter that does not parse, so whether Claude Code keeps the gate cannot be known: it may load at session start; quote the value YAML cannot read (AR306)",
		it.kind, itemID(it.kind, it.cf), quoteAll(keys))
}

// reportWrongTypedGates reports the gate keys of a block that parses whose
// values have the wrong type: `paths` that is not a string or a list of
// strings, `disable-model-invocation` that is not true or false (`yes` is a
// string to YAML 1.2). The finding is on the first such key.
func reportWrongTypedGates(r *runner, it *item, fm frontmatter) {
	var keys []string
	line := 0
	for _, k := range fm.keys {
		bad := false
		switch k.Name {
		case gateKeyPaths:
			_, ok := fmsplit.StringList(k.Value)
			bad = !ok
		case gateKeyDisable:
			_, ok := k.Value.(bool)
			bad = !ok
		}
		if !bad || slices.Contains(keys, k.Name) {
			continue
		}
		keys = append(keys, k.Name)
		if line == 0 {
			line = k.Line
		}
	}
	if len(keys) == 0 {
		return
	}
	r.add(CodeExposureGateUnreadable, it.abs, line,
		"%s %q sets %s to a value of the wrong type (paths takes a string or a list of strings, disable-model-invocation true or false), so the gate cannot be read and the item may load at session start; write the value in that form",
		it.kind, itemID(it.kind, it.cf), quoteAll(keys))
}

// quoteAll is keys quoted and joined by ", ".
func quoteAll(keys []string) string {
	quoted := make([]string, len(keys))
	for i, k := range keys {
		quoted[i] = strconv.Quote(k)
	}
	return strings.Join(quoted, ", ")
}

// gateKeysIn lists the gate keys a frontmatter block names, in the order they
// appear, and the 1-based line of the first. A block that never closes runs to
// the end of the file, as the exposure inventory reads it.
func gateKeysIn(d doc) (keys []string, line int) {
	end := len(d.lines)
	if d.bodyStart > 0 {
		end = d.bodyStart - 1
	}
	gateKeys := fmsplit.GateKeys()
	for i := 1; i < end; i++ {
		for _, key := range gateKeys {
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
