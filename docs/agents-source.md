# .agents as a source

ai-rulez reads its content from the `.ai-rulez` tree by default. With `source = "agents"` it reads the open standards
instead: the root `AGENTS.md` and the `.agents` directory. Teams that already keep their knowledge there can add
ai-rulez as the translator to every other tool without moving a file.

```toml
source = "agents"   # default: "ai-rulez"
```

## Standards and extensions

Future ai-rulez versions will always follow the [AGENTS.md](https://agents.md) and
[Agent Skills](https://agentskills.io) standards. What the standards define is read as the standards define it. What
they do not define is an ai-rulez extension: it lives in folders named after what they hold, and a future version of a
standard supersedes the extension of the same purpose.

| Content                               | Location                                         | Defined by                                  |
| ------------------------------------- | ------------------------------------------------ | ------------------------------------------- |
| Project instructions                  | root `AGENTS.md`                                 | AGENTS.md standard                          |
| Skills, with `references/`, `scripts/` and `assets/` | `.agents/skills/<name>/SKILL.md`  | Agent Skills standard                       |
| Rules                                 | `.agents/rules/<name>.md`                        | Antigravity convention, read by ai-rulez    |
| Agents (subagents)                    | `.agents/agents/<name>.md`                       | Antigravity and Amp convention              |
| Review checks                         | `.agents/checks/<name>.md`                       | Amp convention                              |
| Commands                              | `.agents/commands/<name>.md`                     | ai-rulez extension                          |
| Context files                         | `.agents/context/<name>.md`                      | ai-rulez extension                          |
| Domains                               | `.agents/domains/<name>/{rules,context,skills,agents,commands,checks}` | ai-rulez extension     |

Hooks, MCP servers, permissions, presets, profiles and every other setting stay in the TOML configuration: no
standard defines them. The format of each rule, skill, agent, command and check file is the one described in
[Configuration](configuration.md#content-layout).

## Where the configuration lives

The configuration can sit in either place:

- `.agents/ai-rulez.toml`: preferred when it exists. It implies `source = "agents"`. Everything ai-rulez keeps for
  itself goes in `.agents/.ai-rulez/`: the generated manifest, `ai-rulez.lock`, `config.local.toml` and the
  machine-local `local/` tree. Nothing of ai-rulez is loose among the standard's files.
- `.ai-rulez/config.toml`: with `source = "agents"` set in it. The manifest, lock and `local/` stay in `.ai-rulez/`;
  the rules, context, skills and the rest of the `.ai-rulez` content tree are not read.

When both files exist, `.agents/ai-rulez.toml` is used and the other is ignored.

### Layers and the local overlay

The `config.<name>.toml` layers (`default_layers`, `AI_RULEZ_LAYERS`, `AI_RULEZ_PROFILE`), `config.local.toml` and the
`plugins/` and `marketplaces/` definition files are read from the directory that holds the project's state, never from
the standard's `.agents/` folder:

| Configuration in            | Layers and `config.local.toml` in |
| --------------------------- | --------------------------------- |
| `.agents/ai-rulez.toml`     | `.agents/.ai-rulez/`              |
| `.ai-rulez/config.toml`     | `.ai-rulez/`                      |

A `config.<name>.toml` placed beside `.agents/ai-rulez.toml` is not read. `ai-rulez config layers` lists the layers
found in that directory, and an error for a `default_layers` entry without a file names the path to create.

## What is read

- The root `AGENTS.md` becomes one context file named `agents`. As for every content file, its first `# Title`
  heading is dropped when it is rendered: the item is titled `agents` in the generated files. Frontmatter keys no tool
  reads are kept under `metadata`. `list context` and the MCP list tool show it with `AGENTS.md` as its path;
  `remove context agents` refuses, because `AGENTS.md` is your file: edit or delete it by hand. A nested
  `<scope>/AGENTS.md` is not read as source.
- With `agents_md_split = true` the root `AGENTS.md` becomes one context item per H2 heading (see
  [Splitting AGENTS.md](#splitting-agentsmd)). A nested `<scope>/AGENTS.md` is not read as source.
- Everything under `.agents/` in the layout above is scanned like the `.ai-rulez` tree: same file formats, same
  `bundle_exclude`, and the same symlink policy (a symlink must resolve inside the project). The one exception is
  `AGENTS.md` itself: a symlinked `AGENTS.md` is never followed and is not read.
- A file whose header (the first comment, after any frontmatter) is the ai-rulez generated banner is output of an earlier run and is never read as source. A mention of the banner further down, in prose or a code block, does not make a hand-written file output. This is
  checked on every load, so a project that switched from `source = "ai-rulez"` (whose `generate` wrote
  `AGENTS.md` and `.agents/skills`) is not fed its own output. The ignored files are named in a warning; delete them or
  replace them with hand-written files.

## Splitting AGENTS.md

By default the root `AGENTS.md` is one context item named `agents`. Set `agents_md_split = true` to read it in place
as one item per `##` heading, so a large file can be addressed, listed and removed section by section:

```toml
source = "agents"
agents_md_split = true
```

- The text before the first `##` heading (the title and introduction) is the item `agents`, when it is not blank. Each
  section is `agents-<heading slug>`: `## Code Style` is `agents-code-style`. A heading used twice gets `-2`, `-3`.
  A heading inside a code fence does not split. A section keeps its `##` heading line in the rendered text.
- `list context` and the MCP list tool show every item with `AGENTS.md` as its path, before the files of
  `.agents/context`. `remove context agents-code-style` (and the MCP delete tool) deletes that section from
  `AGENTS.md` and leaves every other byte of the file as it was. Without the setting `remove` refuses to touch the
  file. Items are rendered in name order, so a section's place in the generated files follows its name, not its place
  in `AGENTS.md`.
- The setting needs `source = "agents"` (or a configuration in `.agents/ai-rulez.toml`); `validate` rejects it
  otherwise. `AGENTS.md` itself is still never written by `generate`.

## Not covered

- A nested `<scope>/AGENTS.md` is generated for `[[scopes]]` in the `ai-rulez` source and is never written here; as a
  source only the root `AGENTS.md` is read.
- Hooks, MCP servers, permissions, presets, profiles and roles stay in the TOML configuration: no standard defines
  them, and no ai-rulez extension folder exists for them.

## Validating

`ai-rulez validate` checks `source` (`"ai-rulez"` or `"agents"`; `source = "ai-rulez"` contradicts a configuration in
`.agents/ai-rulez.toml`) and the content it reads.

## Starting a project

- `ai-rulez init --source agents` writes `.agents/ai-rulez.toml` and, when the project has none, a starter `AGENTS.md`.
  It creates no `.ai-rulez` tree and no example content: the content directories belong to the standards. With
  `--config-dir .ai-rulez` the configuration goes to `.ai-rulez/config.toml` with `source = "agents"` instead. An
  existing configuration is only replaced with `--force`, which keeps the old file as `<file>.bak-<timestamp>`; the
  `.agents` directory itself is never moved aside. `init` warns at once when the file it writes would be shadowed by an existing `.agents/ai-rulez.toml`, or would shadow an existing `.ai-rulez/config.toml` (`doctor` reports the same later). The MCP tool `init_project` takes the same `source` argument.
- `ai-rulez init --source agents --from CLAUDE.md,.cursor` and `ai-rulez convert --into .agents` move other tools'
  files into the layout above. What the agents source already reads where it is (the root `AGENTS.md`, everything
  under `.agents`) is left in place and reported as such, the configuration goes to `.agents/ai-rulez.toml` and
  personal content to `.agents/.ai-rulez/local`. The files the tools wrote are never modified; the first `generate`
  replaces them.
- To go back to the `.ai-rulez` tree, set `source = "ai-rulez"` in `.ai-rulez/config.toml` and move the files back (or
  run `convert --from native` over the generated tool files).

## What generate writes

`generate` translates the source to every tool except `.agents` itself. It drops every output that would land on
`AGENTS.md`, a scope's `AGENTS.md`, or anything under `.agents/` (the skills tree, and the `.agents/rules`,
`.agents/checks` and settings files some presets write), and says so in one info line. The files in those places are
yours: a second `generate` leaves every byte of them alone, and `generate --check` stays clean.

- Every other tool gets its full generated files, as with `agents_md = false`: `CLAUDE.md`, `GEMINI.md`, the Cursor,
  Copilot and other rules folders and instruction files carry your always-on rules and context, and skills, agents
  and commands go to each tool's own folders. `agents_md` is forced off for this source, because the shared
  `AGENTS.md` it relies on is yours and is never written; `agents_md = true` is rejected by `validate`.
- The text of your root `AGENTS.md` is one of those context items (`agents`), so a tool without native `AGENTS.md`
  support, such as Claude Code or Gemini, gets it inside its own file. A tool that loads `AGENTS.md` itself (Codex,
  Cursor, Copilot and the others in the [tool matrix](agents-md.md)) is left out of that item, so it never sees the
  text twice. Gemini CLI is not one of them in this source: `agents_md` is off, so its settings list only `GEMINI.md`, and `GEMINI.md` carries the text. A root file two presets share (`.github/copilot-instructions.md` for `copilot` and `copilot-cli`) omits the text only when every configured preset writing it reads `AGENTS.md`. Such a tool receives your rules, agents and commands only where it has a place for them: Codex has no
  rules folder, so it reads your `AGENTS.md` and `.agents/skills` and nothing more. Put what every tool must follow
  in `AGENTS.md`.
- The rule holds through symlinks: an output below a symlinked directory that leads into `.agents` (`.claude/rules ->
  ../.agents/rules`, `.claude/skills/review -> ../../.agents/skills/review`) is dropped with a warning naming it, so
  generate never writes through the link into the source.
- `generate --plugin` follows the same rule: a plugin output that would land on `AGENTS.md` or under `.agents/`
  (the Codex index `.agents/plugins/marketplace.json` from `[plugin.codex] marketplace = true`, or a bundle whose
  `output_dir` is inside `.agents`) is skipped with a warning naming the path, so a hand-written file is never
  overwritten. Point the plugin output elsewhere, or keep that file by hand.
- The generated header's `Source:` line and its "modify ... content" text name the file and folder you edit
  (`.agents/ai-rulez.toml` or `.ai-rulez/config.toml`, and `.agents/`), as does the warning that lists the new hook and
  MCP commands a run will write.
- A generated header in `AGENTS.md` or a file under `.agents` marks it as output of an earlier run: it is never read
  as source, and `generate` retires it like any generated leftover.

## Commands

- `clean` removes what `generate` wrote and never `AGENTS.md` or anything under `.agents`, not even with
  `--include-edited` and whatever an old manifest lists.
- `lock` and the lock checks pin `AGENTS.md` and the `.agents` tree; `ai-rulez.lock` is written to
  `.agents/.ai-rulez/` (or beside the `.ai-rulez/config.toml`).
- `migrate okf` refuses a project with this source (it converts the `.ai-rulez` tree, which is not read), and the
  notice that a `.ai-rulez` tree uses the legacy layout is not shown for it.
- `generate --watch` watches `AGENTS.md` and `.agents`.
- `add`, `remove` and `list` (and the MCP create, update and delete tools) work on `.agents/rules`,
  `.agents/skills`, `.agents/domains/<name>` and the rest; the machine-local variants keep their files in the state
  directory (`.agents/.ai-rulez/local`), which is gitignored. Profile, include and installed-skill changes are
  saved to the configuration file in use.
- `doctor` warns about a `.ai-rulez/config.toml` that `.agents/ai-rulez.toml` shadows, about `.ai-rulez` content
  that `source = "agents"` does not read, and about generated files left in the source.
- The `[guard]` hook keeps agents out of generated files only: `AGENTS.md` and `.agents` stay editable, and its
  message points at them.
- `generate --recursive`, `validate --recursive` and the MCP recursive tools find a nested `.agents/ai-rulez.toml`.
