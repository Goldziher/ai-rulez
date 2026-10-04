# Custom Presets

Set `presets` in `.ai-rulez/config.toml` to control which tools ai-rulez generates for. Most projects
use only built-in presets; custom presets cover a tool ai-rulez does not ship.

## Built-in Presets

| Preset         | Output                                             |
| -------------- | -------------------------------------------------- |
| `claude`       | `CLAUDE.md` and `.claude/` (`rules/`, `skills/`, `agents/`) |
| `cursor`       | `.cursor/rules/`, `.cursor/commands/`, `.agents/`  |
| `gemini`       | `GEMINI.md`, `.gemini/`, `.agents/`                |
| `copilot`      | `.github/copilot-instructions.md`, `.github/instructions/`, `.github/{skills,agents,commands}/` |
| `windsurf`     | `.windsurf/`                                       |
| `continue-dev` | `.continue/`                                       |
| `cline`        | `.clinerules/`, `.cline/`                          |
| `codex`        | `AGENTS.md`, `.agents/skills/` and `.codex/`       |
| `amp`          | `AGENTS.md` and `.agents/` (`.amp/settings.json` when an effort resolves) |
| `junie`        | `.junie/` (`guidelines.md`, `rules/`, `skills/`, `agents/`) |
| `opencode`     | `AGENTS.md`, `.opencode/`, `opencode.json` (when MCP servers are set) |
| `hermes`       | `.hermes.md`                                       |
| `antigravity`  | `.agents/` (`rules/`, `skills/`, `agents/`), `GEMINI.md` |
| `xum`          | `AGENTS.md` and `.xum/`                            |
| `pi`           | `AGENTS.md`, `.agents/skills/`, `.pi/agents/`, `.pi/mcp.json` |
| `baz`          | `AGENTS.md` (root and nested), `.agents/skills/`, `.claude/agents/` (see [Baz](baz.md)) |

Under the default `[rules] mode = "split"` each rule is written to the tool's rules folder
(`.claude/rules/`, `.github/instructions/`, `.junie/rules/`, `.agents/rules/`, ...) and the root file
keeps context. See [Rules](rules.md#rules-mode) for the per-preset behaviour in each mode.

`mcp` is a shared utility preset (the generic `.mcp.json`) invoked automatically when MCP servers are
configured; you normally do not name it.

## Custom Presets

A custom preset is an inline table in the `presets` array (you can mix it with built-in names). It
has a `name`, a `type` (`markdown`, `directory`, or `json`), and a `path`:

```toml
presets = [
  "claude",
  { name = "my-tool", type = "markdown", path = "docs/MY_TOOL.md" },
]
```

A config that defines only custom presets can use `[[presets]]` instead:

```toml
[[presets]]
name = "my-tool"
type = "markdown"
path = "docs/MY_TOOL.md"
```

!!! note "For a full tool, use a provider spec"
    The `markdown`/`directory`/`json` types are intentionally small. If the tool needs skills, agents,
    commands, per-agent frontmatter, effort/model, or an MCP sidecar — the same feature set as a
    built-in preset — reference a declarative **provider spec** with `provider = "…"` instead. See
    [Provider-backed Presets](configuration.md#provider-backed-presets-full-parity).

### Preset types

**`markdown`** — renders one file at `path`. Without a `template`, ai-rulez renders a default
document with a title and the rule and context sections.

```toml
[[presets]]
name = "dev-guide"
type = "markdown"
path = "docs/AI_DEVELOPMENT_GUIDE.md"
```

**`directory`** — creates `path` and writes one markdown file per rule, named after the sanitized
rule name (`<name>.md`). Rules are the only content kind this type emits.

```toml
[[presets]]
name = "agent-rules"
type = "directory"
path = ".my-tool/rules"
```

**`json`** — renders one JSON file at `path`. Without a `template`, ai-rulez marshals the template
data (below) as indented JSON.

```toml
[[presets]]
name = "config-json"
type = "json"
path = "config/rules.json"
```

### Templates

`markdown` and `json` presets accept a `template`, parsed with Go's
[`text/template`](https://pkg.go.dev/text/template). **Only Go's built-in template functions are
available** (`len`, `printf`, `eq`, `index`, `range`, `with`, `if`, …) — there is no extra function
library, so helpers such as `where`, `truncate`, `sortByPriority`, or `now` are not defined and a
template using them fails to parse.

```toml
[[presets]]
name = "my-tool"
type = "markdown"
path = "docs/MY_TOOL.md"
template = """
# {{ .Name }}

{{ range .Rules }}
## {{ .Name }}{{ if .Priority }} ({{ .Priority }}){{ end }}

{{ .Content }}
{{ end }}
"""
```

### Template data

The template receives:

| Field         | Contents                                                        |
| ------------- | --------------------------------------------------------------- |
| `.Name`       | Project name                                                    |
| `.Description`| Project description                                             |
| `.Version`    | Config version                                                  |
| `.Rules`      | Root rules                                                      |
| `.Context`    | Root context                                                    |
| `.Skills`     | Root skills                                                     |
| `.Domains`    | Map of domain name → `{ Name, Rules, Context, Skills }`         |

Each rule/context/skill entry is a map with `Name` and `Content`, plus `Priority` and `Targets` when
the source frontmatter sets them.

## Targeting content at a preset

Content is filtered to a preset with the file's frontmatter `targets`, not with inline config. A rule
with `targets: ["CLAUDE.md"]` is only rendered into outputs matching that target, including rules
folders and every root file; a rule with no `targets` goes to every output. A target can be a preset
name, root file, path, directory prefix, glob, or `*`; see [Targets](rules.md#targets).

```markdown
---
priority: high
targets: ["CLAUDE.md", ".cursor/rules/*"]
---

Guidance that only Claude and Cursor should see.
```

## Ordering

Rules and context render in priority order (critical → high → medium → low → minimal) within each
section, with name order breaking ties. Pinning a `priority` in frontmatter is the supported way to
influence it.

## Combining built-in and custom

Because `presets` is one array, list built-ins and custom presets together:

```toml
presets = [
  "claude",
  "cursor",
  { name = "internal-guide", type = "markdown", path = "docs/INTERNAL.md" },
]
```

## Troubleshooting

**Template fails to parse.** A referenced function is not one of Go's built-ins (see above). Remove
it or rewrite the logic with `range`/`if`.

**Output path is wrong.** `path` is relative to the project root and must stay inside it.

**Content is empty.** Rules may be filtered out by `targets`; remove the frontmatter for a rule that
should appear everywhere.

## Next Steps

- **[Configuration](configuration.md)**: full config reference, including provider specs
- **[Domains & Profiles](domains.md)**: organizing content by team or service
