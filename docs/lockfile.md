# Lock File

`ai-rulez.lock` (in the configuration directory, committed) is the record of exactly what your AI configuration
is made of. It pins three things:

1. **Remote sources**: every git include and installed skill, by commit and tree digest (as before).
2. **Authored content**: a `sha256` digest of every rule, context file, skill (with its resources), agent,
   command, hook and role, with its `id`, `domain`, `owner` and `version` when the frontmatter has them.
3. **Generated outputs**: a digest of each generated file, so a change to what agents are actually told shows up
   in review even when nobody touched a source.

One file, one `tree` digest over all of it. Format version 2; version 1 locks still load (and carry no content pins).

## Threat model

Skills, rules and hooks are instructions an agent follows and, for hooks and skill scripts, code it runs. That makes
them a supply chain:

- a **remote include or skill** can move (a branch is force-pushed, a maintainer account is compromised);
- a **skill resource** (a script under `scripts/`, a file under `assets/`) can change without its `SKILL.md`
  changing, and a `SKILL.md` diff may be a small part of a large pull request;
- a **merge** or a **dependency bump** can alter the rendered output in a way nobody read.

What the lock gives you:

| It detects | How |
| --- | --- |
| A remote source serving different bytes than you reviewed | commit and tree digest (unchanged from lock version 1) |
| An added, removed or edited rule / skill / resource / hook / role | per-item digest, named in the check output |
| An executable bit added to a script | the file mode is part of the digest |
| A change in what gets generated, by any cause | output digests |
| A hand-edited lock line | the `tree` digest no longer matches the pins |

What it does **not** do: it does not say *who* published a change, it does not sandbox anything, and it cannot
tell a malicious edit from a good one. It makes every change explicit and reviewable; a human still reviews it
(see [Reviewing lock diffs](#reviewing-lock-diffs)). Pair it with `ai-rulez scan` / `validate --strict` for the
content itself. Signature or attestation verification is not implemented.

## What is pinned

| `kind` | id | Pinned files |
| --- | --- | --- |
| `rule`, `context`, `agent`, `command` | the item name | the source file (a command with resources also pins them) |
| `skill` | the skill directory name | `SKILL.md` and every loaded resource (`references/`, `scripts/`, `assets/`) |
| `hook` | `<event>:<matcher or *>:<n>` | the `[[hooks]]` group as declared and each `script` file |
| `role` | the role name | the `[[roles]]` entry as declared |
| `settings` | `permissions`, `claude-managed` | the `[permissions]` and `[claude.settings.managed]` sources |

Content from remote includes and built-in packs is not listed item by item: includes are pinned by their own
digest, built-ins by the ai-rulez version. Content from a local-path include outside the configuration directory is
not pinned.

Outputs are pinned from the in-memory rendering, before the `Content-Hash` / `Source-Hash` lines are injected and
with the `Generated:` stamp removed, so the digests are the same under every `[header] hashes` mode and whether or
not `[header] timestamp` is on. Not pinned: machine-local outputs, outputs that may carry resolved secrets, and
documents that are partly yours (the merged `.claude/settings.json`); the latter is pinned through its sources: the
hooks, permissions, managed settings and roles above.

```toml
version = 2
hash_version = 1
ai_rulez_version = "4.25.0"
scope = "all"
outputs_pinned = true
tree = "sha256:6cd1d810fce0e91263b3ebfa3610821a820a07b415a8f2a4b9415de191b6c24e"

[[include]]
name = "shared"
source = "https://github.com/acme/ai-rules"
ref = "main"
commit = "0f3e…"
digest = "sha256:…"

[[item]]
kind = "skill"
id = "deploy"
domain = "backend"
path = "domains/backend/skills/deploy"
digest = "sha256:262d721306783b4c3b554a1345253c266f6f991733dad6a347e1b55d5e57ac05"
owner = "platform"
version = "1.2.0"

[[output]]
path = ".claude/skills/deploy/SKILL.md"
digest = "sha256:…"
```

The file is written deterministically: entries sorted, no timestamps, nothing that depends on map order, on the
operating system or on the machine.

## Hashing scheme (`hash_version = 1`)

All digests are **SHA-256**, written `sha256:<64 hex digits>`. The scheme is frozen by `hash_version`; any change
to it bumps that number and `lock --check` refuses a newer scheme than it understands.

Notation: `lp(x)` is the 8-byte big-endian length of `x` followed by `x`; `u64(n)` is `n` as 8 bytes big-endian.

**File leaf** (one file of an item):

```text
leaf = SHA256( lp("ai-rulez/file/v1") || lp(path) || lp(mode) || lp(data) )
```

- `path` is relative to the item, `/`-separated, with no `.`, `..`, empty segment or backslash.
- `mode` is the string `100755` if any execute bit of the file is set, else `100644`. Nothing else about the file
  mode matters.
- `data` is the **raw bytes on disk**, never the frontmatter-stripped text the loader keeps in memory. For files
  with a text extension (`.md .markdown .mdc .mdx .txt .toml .yaml .yml .json .jsonc .sh .bash .zsh .py .js .mjs
  .cjs .ts`) `CRLF` is converted to `LF` first, so a Windows checkout with `autocrlf` pins the same digest. A lone
  `CR` is kept. Every other file (images, binaries, extensionless files) is hashed byte for byte.

**Item tree** (domain-separated per kind: `rule`, `context`, `skill`, `agent`, `command`, `hook`, `role`,
`settings`, `output`):

```text
digest = SHA256( lp("ai-rulez/<kind>/v1") || u64(n) || leaf_1 || … || leaf_n )
```

with the `n` leaves (32 bytes each) sorted by `path`, bytewise. Duplicate paths are an error. A single-file item is
a tree of one leaf named after the file (`SKILL.md`, `style.md`); a declared item (a hook, a role, a settings
source) is a leaf holding its compact JSON encoding (struct fields in declaration order, map keys sorted), plus one
leaf per hook script.

**Top-level `tree`**:

```text
tree = SHA256( lp("ai-rulez/tree/v1") || u64(n) || { lp(kind) || lp(key) || lp(digest) }… )
```

over every pin, sorted by `(kind, key)`, where `kind`/`key` are `item/<kind>` with `<domain> NUL <id>`, `output`
with the path, `include` and `installed-skill` with the name and `<commit> <digest>`, and `digest` is the
`sha256:<hex>` text of the pin.

Two items with the same kind, domain and id get a `#2` suffix on the second (in path order), so every key is unique.

### Test vectors

These were computed independently (Python's `hashlib`) and are checked by the test suite.

| Input | Digest |
| --- | --- |
| leaf `SKILL.md`, mode `100644`, data `# Title\nbody\n` | `cbac10c14a090ab0301ad17b0013e8c960c5960d5574dcabf38867b98987c621` (hex, no prefix) |
| tree `rule`: `style.md` = `# Style\n` | `sha256:f9c0b1ef53a34e543828ff3459f4f117e08edc2976c22a3ee32d0a65ee212c9c` |
| the same with `# Style\r\n` | the same digest (CRLF normalized) |
| tree `context`: `empty.md` = (empty) | `sha256:6e09fc1d734e8a2db85a25265407b4e78e44e45e712d4f60d2682c121f95c40b` |
| tree `rule` with no files | `sha256:c36b2dc178586a67dfcf7bc1e18ab82e0aa267eb5c5a7d7cba0a6ee7b062ec26` |
| tree `skill`: `SKILL.md` = `---\nname: deploy\n---\nDeploy.\n` (`100644`), `scripts/run.sh` = `#!/bin/sh\necho hi\n` (`100755`), `assets/logo.bin` = bytes `00 01 0d 0a 02` (`100644`) | `sha256:262d721306783b4c3b554a1345253c266f6f991733dad6a347e1b55d5e57ac05` |
| the same skill with `run.sh` at `100644` | `sha256:aea011299a41f59be23cee1a602ab3c03fb3e60aa6ce4dc31f947255925d5c73` |
| top digest of `(item/skill, "\0deploy", <the skill digest above>)` and `(output, "CLAUDE.md", "sha256:" + "ab"×32)` | `sha256:6cd1d810fce0e91263b3ebfa3610821a820a07b415a8f2a4b9415de191b6c24e` |
| top digest of no entries | `sha256:e8c93a22e1ed47e16dd881a55dc4fbc5ba685af20083b93469265da901784029` |

The remote-source `digest` of an include or installed skill keeps the lock-version-1 algorithm (a `sha256` over the
sorted file list, each with its path, executable flag and content hash), so existing pins stay valid.

## Commands

```bash
ai-rulez lock                 # pin remotes (network) and content; writes ai-rulez.lock
ai-rulez lock --content-only  # re-pin authored content and outputs only; offline, remote pins kept
ai-rulez lock shared          # re-pin one include or skill; content pins are kept as they are
ai-rulez lock --check         # verify everything, offline; exit 2 on any difference
ai-rulez lock --diff          # show what `ai-rulez lock` would change; exit 0
ai-rulez lock --diff --format json
ai-rulez generate --locked    # also fail when an authored source differs from the lock
ai-rulez generate --frozen    # --locked without the network
```

`lock` writes deterministically: running it twice produces the same bytes. Its output profile is `--profile`, else
the profile recorded in the lock, else the configured default.

`generate` **never writes the lock**. Accepting a change is always an explicit `ai-rulez lock`.

### `lock --check`

Offline. It compares the remote pins with the local cache (as before), every authored item with its pin, and the
rendered outputs with the output pins, and exits `2` after naming each difference:

```text
ai-rulez.lock does not match /work/app/.ai-rulez:
  source added    rule security (rules/security.md)
  source changed  skill backend/deploy (domains/backend/skills/deploy)
  source removed  hook PreToolUse:Bash:0
  output changed  output .claude/skills/deploy/SKILL.md
run `ai-rulez lock` to refresh it (after reviewing the change with `ai-rulez lock --diff`)
```

`source` lines say an authored item changed; `output` lines say what agents see changed. A source change normally
brings output changes with it, but an output can change alone (a new ai-rulez release, a different include
revision), and that is worth a look too. Exit codes: `0` in sync, `1` the command could not run, `2` differences.

A change of the ai-rulez version is a note, not a failure: output digests can differ between releases, and the
output lines then tell you which.

A lock written before content pins existed (`version = 1`, or no `hash_version`) has none to compare. `--check`
passes with a note unless `[lock] enforce = true`, in which case it fails and asks for `ai-rulez lock`.

If remote includes are configured but not in the local cache, outputs cannot be rendered as `generate` would; the
check then compares sources only and says so.

### `lock --diff`

The same comparison, printed for a pull request description or a bot, and always exit code `0`. `--format json`
prints the document described by
[`schema/lock-diff.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/lock-diff.schema.json):

```json
{
  "schema_version": 1,
  "in_sync": false,
  "lock_version": 2,
  "hash_version": 1,
  "changes": [
    { "scope": "source", "change": "changed", "kind": "skill", "id": "deploy", "domain": "backend",
      "path": "domains/backend/skills/deploy", "old": "sha256:…", "new": "sha256:…", "detail": "version \"1.0.0\" -> \"1.1.0\"" },
    { "scope": "output", "change": "changed", "path": ".claude/skills/deploy/SKILL.md", "old": "sha256:…", "new": "sha256:…" }
  ]
}
```

`scope` is `source`, `output`, `remote` (a remote pin no longer matches) or `lock` (the lock itself: hand-edited
pins, or written with different `[lock]` settings).

## Configuration

```toml
[lock]
enforce = false          # true: strict validation reports drift, and --check requires content pins
include_outputs = true   # false: pin sources only
scope = "all"            # "skills": pin only skills, remote includes and installed skills
```

`scope = "skills"` is for projects that only care about the skill supply chain; it implies no output pins. The
lock records the settings it was written with, and `--check` reports a mismatch so a changed `[lock]` table cannot
silently weaken a check.

With `enforce = true`, and only when a lock exists, `validate --strict` adds:

| Code | Meaning |
| --- | --- |
| `AR981` `lock-source-drift` | An authored item was added, removed or changed since the lock was written (or the lock has no content pins). |
| `AR982` `lock-output-drift` | A generated output differs from its pinned digest. |

Both default to `error`; they can be tuned with `[lint.severity]` like any other code.

## CI usage

Run the lock check next to the drift check:

```yaml
- run: ai-rulez lock --check          # pins match: sources, outputs, remote cache
- run: ai-rulez generate --check      # the committed output matches the sources
- run: ai-rulez generate --locked     # fail if a source changed without a lock update
```

They answer different questions. `generate --check` asks "was the output regenerated after the sources changed?".
`lock --check` asks "was the change *approved*?": a source edit, a new skill or a moved include has to come with a
lock update, which is a separate, small, reviewable diff. On a pull request, `ai-rulez lock --diff` gives a
summary to paste into the description.

## Reviewing lock diffs

The lock holds one small block per item, sorted by kind, domain and id, so `git diff ai-rulez.lock` reads like a
change list:

- a new `[[item]]` block is a new rule, skill, hook or role: open the file at `path`;
- a changed `digest` with an unchanged `version` on a skill with a `version` is a review smell: the content moved
  but nobody bumped the version;
- a changed `owner` line is a change of responsibility;
- a new `[[item]]` of `kind = "hook"` or a changed hook is code that will run on developers' machines: read the
  script;
- an executable skill script that appears or changes is reviewed like any script;
- `[[output]]` churn with no `[[item]]` change means the rendering changed (a release, an include): check why.

The item digest does not tell you what changed; it tells you *that* it did. Pair it with the normal source diff.
A reviewer who sees a lock change with no matching source change should ask why.

## Composing with roles

[Roles](roles.md) are pinned as items (`kind = "role"`), so a new role or a changed `include` / `exclude` list is a
lock change. Per-role outputs are not pinned: the lock pins the default rendering (one profile), because a role
renders a different tree per person. `generate --locked --role <name>` still verifies that every *source* matches
the lock before it generates the role's slice, so a person's role output comes only from reviewed sources. The
`catalog` command reports, per item, its digest and whether the sources are in sync with the lock.
