package config

// Harness names accepted in HookGroup.Targets and HookGroup.Matchers beyond the
// five of settings_authoring.go. Each is the name of the preset (or provider
// spec) that renders the harness's native hooks file; see docs/settings.md for
// the vendor documentation each format was read from.
const (
	HarnessCopilotCLI  = "copilot-cli"
	HarnessFactory     = "factory"
	HarnessAntigravity = "antigravity"
	HarnessQwen        = "qwen"
	HarnessAugment     = "augment"
	HarnessCodeBuddy   = "codebuddy"
	HarnessQoder       = "qoder"
	HarnessCommandCode = "commandcode"
	HarnessLetta       = "letta"
	HarnessGitLabDuo   = "gitlab-duo"
	HarnessDevin       = "devin"
	HarnessGrok        = "grok"
	HarnessBob         = "bob"
	HarnessCortex      = "cortex"
	HarnessGoose       = "goose"
	HarnessDeepAgents  = "deepagents"
	HarnessJunie       = "junie"
	HarnessZCode       = "zcode"
	HarnessCrush       = "crush"
	HarnessPoolside    = "poolside"
	HarnessReasonix    = "reasonix"
	HarnessHermes      = "hermes"
	HarnessKiro        = "kiro"
	HarnessVibe        = "vibe"
	HarnessKimi        = "kimi"
	HarnessCline       = "cline"
)

// extraHookHarnesses are the harnesses after the first five in HookHarnesses.
var extraHookHarnesses = []string{
	HarnessCopilotCLI, HarnessFactory, HarnessAntigravity, HarnessQwen, HarnessAugment,
	HarnessCodeBuddy, HarnessQoder, HarnessCommandCode, HarnessLetta, HarnessGitLabDuo, HarnessDevin,
	HarnessGrok, HarnessBob, HarnessCortex, HarnessGoose, HarnessDeepAgents, HarnessJunie, HarnessZCode,
	HarnessCrush, HarnessPoolside, HarnessReasonix, HarnessHermes, HarnessKiro, HarnessVibe, HarnessKimi, HarnessCline,
}
