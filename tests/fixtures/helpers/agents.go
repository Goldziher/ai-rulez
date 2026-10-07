package fixtures

// V4ModelByPresetConfigTOML exercises the per-preset model override chain:
// agent `<preset>_model` frontmatter wins, otherwise `defaults.model_by_preset`
// for that preset, otherwise the legacy `model:` field.
const V4ModelByPresetConfigTOML = `version = "5.0"
name = "model-by-preset-test"
description = "E2E coverage for per-preset model overrides"
gitignore = false
builtins = false

presets = ["claude", "copilot"]

[defaults.model_by_preset]
claude = "haiku"
copilot = "gpt-5"
`

// V4AgentWithPerPresetModels carries explicit per-preset model overrides plus a
// legacy `model:` field. The per-preset entries must win over the legacy field
// in their matching preset output.
const V4AgentWithPerPresetModels = `---
name: agent-overrides
description: Agent that pins a different model per preset
model: sonnet
claude_model: opus
copilot_model: gpt-4
tools:
  - Read
---

Pinned per-preset.
`

// V4AgentWithDefaultedModel carries no model frontmatter at all, so each preset
// must fall through to its `defaults.model_by_preset` entry.
const V4AgentWithDefaultedModel = `---
name: agent-defaulted
description: Agent with no model frontmatter
tools:
  - Read
---

Falls through to defaults.model_by_preset.
`
