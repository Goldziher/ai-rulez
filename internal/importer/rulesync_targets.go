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
	"antigravity-cli":    litAntigravity,
	"antigravity-ide":    litAntigravity,
	"augmentcode":        "augment",
	"augmentcode-legacy": "augment",
	"bob":                "bob",
	"claudecode":         litClaude,
	"claudecode-legacy":  litClaude,
	"cline":              "cline",
	"codebuddy":          "codebuddy",
	"codebuff":           "codebuff",
	"codewhale":          "codewhale",
	"codexcli":           litCodex,
	"commandcode":        "commandcode",
	litCopilot:           litCopilot,
	"copilotcli":         "copilot-cli",
	"cortexcode":         "cortex",
	"crush":              "crush",
	litCursor:            litCursor,
	"deepagents":         "deepagents",
	litDevin:             litDevin,
	"dsh":                "dsh",
	"factorydroid":       "factory",
	"geminicli":          litGemini, // name of older rulesync releases; not in the current target list
	"gitlabduo":          "gitlab-duo",
	"goose":              "goose",
	"grokcli":            "grok",
	"hermesagent":        "hermes",
	litJunie:             litJunie,
	"kilo":               "kilo",
	"kimi-code":          "kimi",
	litKiro:              litKiro,
	"kiro-cli":           litKiro,
	"kiro-ide":           litKiro,
	"lettacode":          "letta",
	"mimocode":           "mimocode",
	"musecode":           "muse",
	"omp":                "omp",
	"openclaw":           "openclaw",
	litOpencode:          litOpencode,
	"pi":                 "pi",
	"pool":               "poolside",
	"qoder":              "qoder",
	"qwencode":           "qwen",
	"reasonix":           "reasonix",
	"replit":             "replit",
	"roo":                litZoocode,
	"rovodev":            "rovodev",
	litTakt:              litTakt,
	"trae":               "trae",
	"vibe":               "vibe",
	litWarp:              litWarp,
	"warpcli":            litWarp,
	"windsurf":           litDevin, // renamed devin upstream
	"zcode":              "zcode",
	"zed":                "zed",
	litZoocode:           litZoocode,
}

// rulesyncUnsupported explains the targets that have no preset.
var rulesyncUnsupported = map[string]string{
	"agentsmd":           "AGENTS.md is written by several presets (codex, opencode, amp, xum, pi); enable one of them or set agents_md",
	"agentsskills":       "the shared .agents/skills output has no preset of its own; any preset that writes it covers it",
	"antigravity-plugin": litNoPluginPreset,
	"augmentcode-plugin": litNoPluginPreset,
	"claudecode-plugin":  litNoPluginPreset,
	"continue":           "Continue is end of life and has no ai-rulez preset",
	"devin-plugin":       litNoPluginPreset,
	"kimi-code-plugin":   litNoPluginPreset,
	"tabnine":            "the legacy Tabnine CLI has no ai-rulez preset",
	"vibe-plugin":        litNoPluginPreset,
	"zcode-plugin":       litNoPluginPreset,
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
			p.add(newFinding(StatusNeedsAction, source, litTargets, litTargets,
				reason+"; the original names were kept in targets, so the file reaches no output until you set targets"))
			return unmapped
		}
		p.add(newFinding(StatusApproximated, source, litTargets, litTargets, reason))
	}
	return out
}
