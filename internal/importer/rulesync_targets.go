package importer

import (
	"sort"
	"strings"
)

// rulesyncPresets maps a rulesync target name to the ai-rulez preset that
// writes the same tool's files. The names come from the rulesync tool-target
// list (src/types/tool-target-tuples.ts). A name that is absent has no
// counterpart; rulesyncUnsupported says why.
var rulesyncPresets = map[string]string{
	"aiassistant":        "aiassistant",
	"amp":                "amp",
	"antigravity-cli":    "antigravity",
	"antigravity-ide":    "antigravity",
	"augmentcode":        "augment",
	"augmentcode-legacy": "augment",
	"bob":                "bob",
	"claudecode":         "claude",
	"claudecode-legacy":  "claude",
	"cline":              "cline",
	"codebuddy":          "codebuddy",
	"codebuff":           "codebuff",
	"codewhale":          "codewhale",
	"codexcli":           "codex",
	"commandcode":        "commandcode",
	"copilot":            "copilot",
	"copilotcli":         "copilot-cli",
	"cortexcode":         "cortex",
	"crush":              "crush",
	"cursor":             "cursor",
	"deepagents":         "deepagents",
	"devin":              "devin",
	"dsh":                "dsh",
	"factorydroid":       "factory",
	"geminicli":          "gemini", // name of older rulesync releases; not in the current target list
	"gitlabduo":          "gitlab-duo",
	"goose":              "goose",
	"grokcli":            "grok",
	"hermesagent":        "hermes",
	"junie":              "junie",
	"kilo":               "kilo",
	"kimi-code":          "kimi",
	"kiro":               "kiro",
	"kiro-cli":           "kiro",
	"kiro-ide":           "kiro",
	"lettacode":          "letta",
	"mimocode":           "mimocode",
	"musecode":           "muse",
	"omp":                "omp",
	"openclaw":           "openclaw",
	"opencode":           "opencode",
	"pi":                 "pi",
	"pool":               "poolside",
	"qoder":              "qoder",
	"qwencode":           "qwen",
	"reasonix":           "reasonix",
	"replit":             "replit",
	"roo":                "zoocode",
	"rovodev":            "rovodev",
	"takt":               "takt",
	"trae":               "trae",
	"vibe":               "vibe",
	"warp":               "warp",
	"warpcli":            "warp",
	"windsurf":           "devin", // renamed devin upstream
	"zcode":              "zcode",
	"zed":                "zed",
	"zoocode":            "zoocode",
}

// rulesyncUnsupported explains the targets that have no preset.
var rulesyncUnsupported = map[string]string{
	"agentsmd":           "AGENTS.md is written by several presets (codex, opencode, amp, xum, pi); enable one of them or set agents_md",
	"agentsskills":       "the shared .agents/skills output has no preset of its own; any preset that writes it covers it",
	"antigravity-plugin": "plugin packaging targets have no ai-rulez preset",
	"augmentcode-plugin": "plugin packaging targets have no ai-rulez preset",
	"claudecode-plugin":  "plugin packaging targets have no ai-rulez preset",
	"continue":           "Continue is end of life and has no ai-rulez preset",
	"devin-plugin":       "plugin packaging targets have no ai-rulez preset",
	"kimi-code-plugin":   "plugin packaging targets have no ai-rulez preset",
	"tabnine":            "the legacy Tabnine CLI has no ai-rulez preset",
	"vibe-plugin":        "plugin packaging targets have no ai-rulez preset",
	"zcode-plugin":       "plugin packaging targets have no ai-rulez preset",
}

// rulesyncTargetFile names the root file a target without a preset selects in an
// item's `targets` (ai-rulez matches root file names as targets).
var rulesyncTargetFile = map[string]string{"agentsmd": "AGENTS.md"}

// mapItemTargets turns the `targets` of one rulesync file into ai-rulez
// `targets`. "*" or an absent list means every output and yields no targets.
// Targets without a counterpart are reported; when none is left the list keeps
// the rulesync names so the content is preserved but reaches no output until
// someone reviews it, instead of silently widening to every tool.
func mapItemTargets(p *Plan, source string, raw []string) []string {
	if len(raw) == 0 {
		return nil
	}
	var out, unmapped []string
	for _, t := range raw {
		t = strings.TrimSpace(t)
		switch {
		case t == "*":
			return nil
		case rulesyncPresets[t] != "":
			out = append(out, rulesyncPresets[t])
		case rulesyncTargetFile[t] != "":
			out = append(out, rulesyncTargetFile[t])
		case t != "":
			unmapped = append(unmapped, t)
		}
	}
	out = dedupeSorted(out)
	sort.Strings(out)
	if len(unmapped) > 0 {
		sort.Strings(unmapped)
		reason := "no ai-rulez preset for " + strings.Join(unmapped, ", ") + "; the file is not written for them"
		if len(out) == 0 {
			p.add(newFinding(StatusNeedsAction, source, "targets", "targets",
				reason+"; the original names were kept in targets, so the file reaches no output until you set targets"))
			return unmapped
		}
		p.add(newFinding(StatusApproximated, source, "targets", "targets", reason))
	}
	return out
}
