# Custom presets

`presets` in `.ai-rulez/config.toml` lists the tools ai-rulez generates for. Most projects use only built-in presets
(`claude`, `cursor`, `copilot`, `codex`, `gemini`, ...; all 52 are listed in [Supported harnesses](harnesses.md)). A
custom preset writes your rules to a file or folder of your choice, for a tool or a document ai-rulez does not ship.

Looking for profiles (named sets of domains)? See [Domains and profiles](domains.md).

## Example

```toml
# .ai-rulez/config.toml
presets = [
  "claude",
  "cursor",
  { name = "review-bot", type = "markdown", path = "docs/REVIEW_BOT.md", template = """
# {{ .Name }} conventions
{{ range .Rules }}
## {{ .Name }}{{ if .Priority }} ({{ .Priority }}){{ end }}

{{ .Content }}
{{ end }}""" },
  { name = "rules-dir", type = "directory", path = ".mytool/rules" },
  { name = "rules-json", type = "json", path = "build/rules.json" },
]
```

With two rules, `commit-messages` (`priority: high`) and `typescript` (`priority: medium`), `ai-rulez generate`
writes the built-in outputs plus:

```markdown
<!-- docs/REVIEW_BOT.md -->
# acme conventions

## commit-messages (high)

Use Conventional Commits.


## typescript (medium)

No `any`.
```

```text
.mytool/rules/
├── commit-messages.md
└── typescript.md
```

`build/rules.json`:

```json
{
  "Context": [],
  "Description": "",
  "Domains": {},
  "Name": "acme",
  "Rules": [
    { "Content": "Use Conventional Commits.\n", "Name": "commit-messages", "Priority": "high" },
    { "Content": "No `any`.\n", "Name": "typescript", "Priority": "medium" }
  ],
  "Skills": [],
  "Version": "5.0"
}
```

## Preset keys

A custom preset is an inline table in the `presets` array, mixed freely with built-in names. A config with only
custom presets can also use `[[presets]]` tables.

| Key        | Type   | Meaning                                                                                  |
| ---------- | ------ | ---------------------------------------------------------------------------------------- |
| `name`     | string | required; identifies the preset in messages; a template can match it against a rule's `Targets` |
| `type`     | string | `markdown`, `directory` or `json`                                                        |
| `path`     | string | output file (`markdown`, `json`) or folder (`directory`), relative to the project root   |
| `template` | string | optional Go template for `markdown` and `json`; see [Templates](#templates)              |
| `provider` | string | path to a provider spec instead of `type` and `path`; see below                          |

`path` must stay inside the project; see [Output paths](#output-paths).

!!! note "For a full tool, use a provider spec"
    The `markdown`, `directory` and `json` types render rules and context only. If the tool needs skills, agents,
    commands, per-file frontmatter, model or effort settings, or an MCP file, the same feature set as a built-in
    preset, point `provider = "..."` at a declarative provider spec instead. See
    [Provider-backed presets](configuration.md#provider-backed-presets-full-parity).

## Preset types

| Type        | Writes                                                                                              |
| ----------- | --------------------------------------------------------------------------------------------------- |
| `markdown`  | one file at `path`. Without a `template`: a title, the project description, and the rule, context and skill sections. |
| `directory` | one `<id>.md` file per rule in the folder `path`, with a `# <id>` heading and the priority line. Only rules are written. |
| `json`      | one file at `path`. Without a `template`: the [template data](#template-data) as indented JSON.      |

```toml
[[presets]]
name = "dev-guide"
type = "markdown"
path = "docs/AI_DEVELOPMENT_GUIDE.md"
```

## Templates

`markdown` and `json` presets take a `template` in Go's [`text/template`](https://pkg.go.dev/text/template) syntax.
Only the built-in template functions are available (`len`, `printf`, `eq`, `index`, `and`, `or`, `not`, ...) and the
actions `range`, `if` and `with`. Any other function fails the run:

```text
Error: generate presets: generate preset review-bot: parse template: template: custom:1: function "now" not defined
```

### Template data

| Field          | Contents                                                                  |
| -------------- | ------------------------------------------------------------------------- |
| `.Name`        | project name                                                              |
| `.Description` | project description                                                       |
| `.Version`     | config version                                                            |
| `.Rules`       | rules of the active profile, root and domains together, in priority order |
| `.Context`     | context files, likewise                                                   |
| `.Skills`      | skills, likewise                                                          |
| `.Domains`     | map of domain name to `{ Name, Rules, Context, Skills }`                  |

Each rule and context entry has `Name` and `Content`, plus `Priority` and `Targets` when the frontmatter sets them
(`Priority` is left out with `compact = true`). A skill entry also has `Body` (the instructions only), `Resources`
(each with `Kind`, `RelPath`, `Content`, `Description`) and its other frontmatter keys; its `Content` includes the
inlined reference files.

## Output paths

The same rules apply to every output path a configuration names: a custom preset `path`, a provider spec's
`root.file`, `outputs.*.dir`, `sidecars[].path` and `directories`, `okf.dir`, `marketplace.output_dir` and
`codex_skills_dir`. `validate` and `generate` reject:

- an empty, absolute or drive- or UNC-prefixed path, and any `..` segment;
- a `.git` segment anywhere in the path (including the Windows spellings `.git.` and `git~1`);
- a path under `.ai-rulez/`, `.config/ai-rulez/`, `.hg/` or `.svn/`.

A preset may write a file that CI or a developer tool runs (`.github/workflows/`, `Makefile`, `.vscode/tasks.json`);
the `generate` summary lists it as `exec-file` before writing.

## Ordering

Rules and context are ordered by priority (`critical`, `high`, `medium`, `low`, `minimal`), then by name. Set
`priority` in a rule's frontmatter to move it.

## Targeting content at a preset

Frontmatter [`targets`](rules.md#targets) limits a rule to some outputs of the built-in presets. The `markdown`,
`directory` and `json` presets do not apply it: they receive every rule of the active profile. A template can filter
on the `Targets` field itself:

```text
{{ range .Rules }}{{ $bot := false }}{{ range .Targets }}{{ if eq . "review-bot" }}{{ $bot = true }}{{ end }}{{ end }}
{{ if or (not .Targets) $bot }}## {{ .Name }}

{{ .Content }}{{ end }}{{ end }}
```

## Built-in presets and rules folders

Under the default `[rules] mode = "split"`, built-in presets with a rules folder (`.claude/rules/`,
`.cursor/rules/`, `.github/instructions/`, ...) get one file per rule, and with `agents_md = true` (the default) the
always-on rules go to the shared `AGENTS.md`. See [Rules](rules.md#rules-mode) for each preset in each mode, and
[AGENTS.md and .agents/skills](agents-md.md).

`mcp` is a utility preset that writes the generic `.mcp.json`. It runs automatically when MCP servers are
configured; you do not need to list it.

## Troubleshooting

| Symptom                                   | Cause and fix                                                                        |
| ----------------------------------------- | ------------------------------------------------------------------------------------ |
| `function "..." not defined`              | The template uses a function that is not a Go built-in. Rewrite it with `range`/`if`. |
| `has an unsafe 'path'`                    | `path` leaves the project or points into a control directory; see [Output paths](#output-paths). |
| The output is missing a rule              | The rule's domain is not in the active profile, or the template filters it out.     |
| A warning that hooks are not generated    | Custom presets have no hook support; hooks are written for built-in presets only.    |
