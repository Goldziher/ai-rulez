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
