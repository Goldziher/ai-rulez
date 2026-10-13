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

## What is read

- The root `AGENTS.md` becomes one context file named `agents`. Frontmatter keys no tool reads are kept under
  `metadata`. A nested `<scope>/AGENTS.md` is not read as source.
- Everything under `.agents/` in the layout above is scanned like the `.ai-rulez` tree: same file formats, same
  symlink policy (symlinks must resolve inside the project), same `bundle_exclude`.
- A file that carries the ai-rulez generated header is output of an earlier run and is never read as source. This is
  checked on every load, so a project that switched from `source = "ai-rulez"` (whose `generate` wrote
  `AGENTS.md` and `.agents/skills`) is not fed its own output. The ignored files are named in a warning; delete them or
  replace them with hand-written files.

## Not covered

- A nested `<scope>/AGENTS.md` is generated for `[[scopes]]` in the `ai-rulez` source and is never written here; as a
  source only the root `AGENTS.md` is read.
- `AGENTS.md` is one context file. Splitting it at its headings (`convert --split-headings` for a copied file) has no
  equivalent for a file that is read in place.
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
  `.agents` directory itself is never moved aside. The MCP tool `init_project` takes the same `source` argument.
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
