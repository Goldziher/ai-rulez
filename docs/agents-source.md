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

## Validating

`ai-rulez validate` checks `source` (`"ai-rulez"` or `"agents"`; `source = "ai-rulez"` contradicts a configuration in
`.agents/ai-rulez.toml`) and the content it reads.

## What generate writes

`generate` translates the source to every tool except `.agents` itself. It drops every output that would land on
`AGENTS.md`, a scope's `AGENTS.md`, or anything under `.agents/` (the skills tree, and the `.agents/rules`,
`.agents/checks` and settings files some presets write), and says so in one info line. The files in those places are
yours: a second `generate` leaves every byte of them alone, and `generate --check` stays clean.

- `CLAUDE.md` is still generated: it is the shim that imports `@AGENTS.md`, so Claude Code reads your file.
- Tools that read `AGENTS.md` and `.agents/skills` themselves (Codex, Cursor, Copilot, Gemini through
  `context.fileName`, and the others in the [tool matrix](agents-md.md)) read the originals and need nothing written.
- Rules, agents, commands and checks keep going to each tool's own folders (`.claude/rules`, `.cursor/rules`, ...).
  An always-on rule or context file that exists only under `.agents/rules` or `.agents/context` has no `AGENTS.md`
  to be inlined into, so it reaches the tools that have a rules folder only when it is scoped. Keep always-on
  instructions in `AGENTS.md`, or set `agents_md = false` to give every tool its own full file (`AGENTS.md` and
  `.agents` are still never written).
- A generated header in `AGENTS.md` or a file under `.agents` marks it as output of an earlier run: it is never read
  as source, and `generate` retires it like any generated leftover.

## Commands

- `clean` removes what `generate` wrote and never `AGENTS.md` or anything under `.agents`, not even with
  `--include-edited` and whatever an old manifest lists.
- `lock` and the lock checks pin `AGENTS.md` and the `.agents` tree; `ai-rulez.lock` is written to
  `.agents/.ai-rulez/` (or beside the `.ai-rulez/config.toml`).
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
