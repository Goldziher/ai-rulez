# Lock File

`ai-rulez.lock` (in the configuration directory, committed) is the record of exactly what your AI configuration
is made of. It pins three things:

1. **Remote sources**: every git include and installed skill, by commit and tree digest (as before).
2. **Authored content**: a `sha256` digest of every rule, context file, skill (with its resources), agent,
   command, hook and role, with its `id`, `domain`, `owner` and `version` when the frontmatter has them.
3. **Generated outputs**: a digest of each generated file, so a change to what agents are actually told shows up
   in review even when nobody touched a source.

One file, one `tree` digest over all of it, under **one hashing scheme** for every kind (authored items, outputs,
remote includes, installed skills, skill sources, OKF includes and served skills). Lock `version = 1`; a lock with
any other version is refused with an instruction to run `ai-rulez lock` again.

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
| A remote source serving different bytes than you reviewed | commit and tree digest |
| An added, removed or edited rule / skill / resource / hook / role | per-item digest, named in the check output |
| An executable bit added to a script | the file mode is part of the digest |
| A change in what gets generated, by any cause | output digests |
| An accidentally edited or truncated lock | the `tree` digest no longer matches the pins, or is missing |
| A lock replaced by one without content pins (a downgrade) | `lock --check` fails and `generate --locked` warns; under `enforce`, `generate --locked` and `validate --strict` fail |

The `tree` digest is an integrity check, not a signature: whoever can edit the lock can recompute it (`ai-rulez
lock` does exactly that). It catches accidental edits and merge mistakes; a deliberate change to the pins is caught
by review of the lock diff and by CI running `lock --check` against the sources, not by the digest.

What it does **not** do: it does not say *who* published a change, it does not sandbox anything, and it cannot
tell a malicious edit from a good one. It makes every change explicit and reviewable; a human still reviews it
(see [Reviewing lock diffs](#reviewing-lock-diffs)). Pair it with `ai-rulez scan` / `validate --strict` for the
content itself. Signature or attestation verification is not implemented.

## What is pinned

| `kind` | id | Pinned files |
| --- | --- | --- |
| `rule`, `context`, `agent`, `command`, `check` | the item name | the source file (a command with resources also pins them) |
| `skill` | the skill directory name | `SKILL.md` and every loaded resource (`references/`, `scripts/`, `assets/`) |
| `local-include` | the include name | the content directories (`rules`, `context`, `skills`, `agents`, `commands`, `checks`, `domains`) of an include whose `source` is a local path; an OKF include is pinned whole |
| `hook` | `<event>:<matcher or *>:<n>` | the `[[hooks]]` group as declared and each `script` file |
| `role` | the role name | the `[[roles]]` entry as declared |
| `settings` | `permissions`, `claude-managed`, `mcp-servers` | the `[permissions]`, `[claude.settings.managed]` and `[[mcp_servers]]` sources (MCP servers as written, placeholders unresolved, including those of a legacy `mcp.yaml`/`mcp.toml`/`mcp.json`) |

Declared configuration that is **not** pinned at the source: profiles, `include` configuration, scoped (monorepo)
configuration, plugin and marketplace authoring, and the machine-local overlay. A change there is caught only through
the output pins, so with `include_outputs = false` (or `scope = "skills"`) it is not covered. Keep output pins on
when you rely on the lock for these.

Content from remote includes and built-in packs is not listed item by item: includes are pinned by their own
digest, built-ins by the ai-rulez version. A local-path include is pinned as one `local-include` item over its
content directories, wherever it lives (inside the repository or outside it). A missing path cannot be pinned:
`lock` warns and `lock --check` fails until it is fixed. A symlink inside the pinned tree is never followed: it is
pinned as its own entry, by link target (see below), and the loader does not read it. A `local_override` path is a
development shortcut and is not pinned.

Outputs are pinned from the in-memory rendering, before the `Content-Hash` / `Source-Hash` lines are injected and
with the `Generated:` stamp removed, so the digests are the same under every `[header] hashes` mode and whether or
not `[header] timestamp` is on. Not pinned: machine-local outputs, outputs that may carry resolved secrets, and
documents that are partly yours, that is, a merged document in which the consumer owns some keys. A plain
`.claude/settings.json` that only ai-rulez writes is pinned like any other output. When the document is partly
yours it is not pinned as a whole; its sources (the hooks, permissions, managed settings, MCP servers and roles
above) are.

```toml
version = 1
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

The file is written deterministically: entries sorted, no timestamps, nothing that depends on map order or on the
machine. Digests are the same on every operating system when the repository records the executable bit (see
[File modes](#hashing-scheme)); a script that is executable on disk but not recorded in git digests differently
on Windows.

## Served skills and skill sources

The same file carries the entries of [dynamic skill loading](mcp-server.md#dynamic-skill-loading), so one `lock`
pins everything and one `lock --check` verifies everything:

```toml
[[source]]            # one per [[skill_sources]] entry: the commit its ref resolved to and the tree digest
name = "vendor"
source = "https://github.com/acme/skills"
ref = "v1.2.0"
path = "skills"
commit = "0f3e…"
digest = "sha256:…"

[[served]]            # one per skill the skills server serves, in the default view and in every other view
name = "deploy"
source = ".ai-rulez/skills/deploy/SKILL.md"
commit = ""
digest = "sha256:…"   # the served-skill digest below

[[served]]            # the same skill in another view: `view` is only written for a view other than the default
name = "deploy"
view = "role:backend"
source = ".ai-rulez/skills/deploy/SKILL.md"
commit = ""
digest = "sha256:…"
```

A served entry is keyed by name and `view`. The view is the way the skills server is started: `role:<name>` for
`--role`, `profile:<name>` for `--profile`, `static` for `--include-static`, and `source:<name>` for each `--source`,
joined by `+` (`role:backend+static`). The default view has no `view` key, so a project that uses no roles or view
flags writes the same lock as before. A plain `ai-rulez lock` pins the default view, every role and every view the
lock already records; `lock --role`, `--profile`, `--include-static` and `--source` add the view they name. The
server and `lock --check` read the pins of the view they run with. A pin without a `view` also covers every view
(locks written before views existed), but its digest must still match. `lock --strict` fails when the security scan
refuses a served skill; without it the skill is left unpinned, the rest is pinned and `lock` exits 3.

A served skill is a tree digest in the scheme below with the kind `served-skill` (`ai-rulez/served-skill/v1`), over
the files the server returns, with the lines of the generated header that change without the skill changing left
out (the project-wide `Source-Hash` and the `Generated:` stamp, comment lines in the first 40 lines only). It
cannot collide with the digest of the authored skill of the same name. There is one implementation
(`contentlock.ServedDigest`); the skills server, `lock`, `lock --check`/`--diff` and `[lock] enforce` all use it,
and the per-file and whole-skill digests the server reports (`digest`) are the same scheme over the bytes as served.
A fetched source tree, a remote include, an OKF include and an installed skill are digested with the same scheme
(`contentlock.DigestDir`: the regular files below the directory as one tree, kinds `include`, `okf-include`,
`installed-skill` and `skill-source`, with `.git` and the root `.cache_meta.json` bookkeeping left out; files are
streamed, not read whole); all entries are covered by `tree`. A symlink in such a tree is never followed and no
longer an error: it is a leaf of its own, `sha256(lp("ai-rulez/symlink/v1") || lp(path) || lp(target))` with the
link target string from `readlink` (`/`-separated), so retargeting the link changes the digest while what it points
at is not read. A non-regular entry that is not a symlink (a Windows junction, a socket, a device) is the leaf
`sha256(lp("ai-rulez/irregular/v1") || lp(path))` and is never read. Trees without such entries digest exactly as
before. The `tree` digest also covers the `source`, `ref` and `path` of every remote entry and the serve `view` of a
served entry, so editing or swapping them is detected even when the content digests do not change.

`lock --check` and `lock --diff` compare these entries without the network: a changed served skill is a `served`
change, a source whose cached tree no longer matches its pin is a `remote` change. `[lock] enforce = true` makes
`validate --strict` report served-skill mismatches as `AR995` and makes the server refuse them. `lock --kind
source|served` refreshes one kind, `generate --frozen`/`mcp --serve-skills --frozen` never use the network, and
`lock --content-only` recomputes authored content and the served digests of local skills offline while keeping the
remote pins.

## Hashing scheme

All digests are **SHA-256**, written `sha256:<64 hex digits>`. The scheme is part of the lock format: any change
to it bumps the lock `version`, and a lock of another version is refused.

Notation: `lp(x)` is the 8-byte big-endian length of `x` followed by `x`; `u64(n)` is `n` as 8 bytes big-endian.

**File leaf** (one file of an item):

```text
leaf = SHA256( lp("ai-rulez/file/v1") || lp(path) || lp(mode) || lp(data) )
```

- `path` is relative to the item, `/`-separated, with no `.`, `..`, empty segment or backslash.
- `mode` is the string `100755` if the file is executable, else `100644`. Nothing else about the file mode
  matters. On Linux and macOS the owner execute bit decides (`0o100`), because git records only that bit
  (`100755` versus `100644`); group and other execute bits are ignored. Windows
  filesystems have no execute bit, so there the mode is taken from the git index (`git ls-files -s`, the mode git
  checks out on Unix); if git is not available or the file is not tracked it is `100644`. A checkout whose
  repository records the executable bit therefore pins the same digest on every operating system. A script that is
  executable on disk but not recorded as such in git will pin differently on Windows than elsewhere; commit the
  bit (`git update-index --chmod=+x`). Remote trees are digested the same way (the mode helper is shared).
- `data` is the **raw bytes on disk**, never the frontmatter-stripped text the loader keeps in memory. For
  documents and data files (`.md .markdown .mdc .mdx .txt .toml .yaml .yml .json .jsonc`) `CRLF` is converted to
  `LF` first, so a Windows checkout with `autocrlf` pins the same digest. A lone `CR` is kept. Every other file
  (images, binaries, extensionless files) and every **script** (`.sh .bash .zsh .py .js .mjs .cjs .ts`) is hashed
  byte for byte: a `CRLF` in a shell script changes behaviour (`#!/bin/sh\r` fails with "bad interpreter"), so a
  script whose only change is its line endings pins a different digest. Keep scripts `LF` in git
  (`.gitattributes`: `*.sh text eol=lf`).

**Item tree** (domain-separated per kind: `rule`, `context`, `skill`, `agent`, `command`, `check`, `hook`, `role`,
`settings`, `output`, `include`, `okf-include`, `installed-skill`, `skill-source`, `served-skill`):

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
with the path, `include`, `installed-skill`, `skill-source` and `served-skill` with the name and `<commit> <digest>`, and `digest` is the
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
| tree `skill`: `run.sh` = `echo hi\n` (`100755`) | `sha256:d7aec0e55512be14e8c20554ee6da9911881211fec26eec3927bb18b21212b54` |
| the same with `echo hi\r\n` (scripts are not normalized) | `sha256:5b9fcff355356346b64a0f8146b7c65b765f377d1deddf29318ef9e686e7ee92` |
| tree `installed-skill`: `SKILL.md` = `# S\n` | `sha256:018df4aac28d9eca573a05cb491c6704407d9b5de8ca10e0dfe20300928140db` |
| tree `skill-source`: `SKILL.md` = `# S\n` | `sha256:cf90b42b8c9bbb2e9bc94075e4d8a183117f45b46d0b746b7bc617587f64bc7b` |
| tree `served-skill`: `SKILL.md` = `# x\n` | `sha256:2225664563ac4bc0affa10d8ca1ea4dcd5ed2a61792b124824005493dffb458c` |
| directory digest, kind `include`: `rules/a.md` = `# A\n` (`100644`), `hooks/x.sh` = `#!/bin/sh\n` (`100755`) | `sha256:94a2c6de5e10aa7eef64adb55330c4a84c02d49566d56adcedbd7f5db2ddf01c` |
| the same directory, kind `okf-include` | `sha256:990b39514f6589c5e61d584b7f59173cb8b1a3bd260ee93af92ebedc819da5e7` |
| top digest of no entries | `sha256:e8c93a22e1ed47e16dd881a55dc4fbc5ba685af20083b93469265da901784029` |

The remote-source `digest` of an include, OKF include, installed skill or skill source is the directory digest above:
there is no second algorithm.

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
revision), and that is worth a look too. Exit codes: `0` in sync, `1` the command could not run, `2` differences,
and for `lock` itself `3` when the lock was written but served skills were left unpinned because the security scan
refuses them (`--strict` fails with `1` instead). Over several roots (`--recursive`) the most severe code wins:
`1`, then `2`, then `3`.

A change of the ai-rulez version is a note, not a failure: output digests can differ between releases, and the
output lines then tell you which.

A lock without content pins (one written by `lock <name>` before any content was pinned) has none to compare, and a
lock whose pins were stripped looks the same. `--check` therefore fails on it (exit `2`) and asks for `ai-rulez lock`,
whatever `enforce` says: a check that passes on such a lock would let a downgrade switch the content checks off.
`generate` keeps using the include and skill pins of such a lock. `generate --locked` on it warns, and fails under `enforce`. A lock with content pins must also carry a `tree` digest.
A hook `script` outside the project cannot be pinned; it is reported as a `lock` change (not an abort) until it
moves inside the project.

If remote includes are configured but not in the local cache, outputs cannot be rendered as `generate` would; the
check then compares sources only and says so in a note, or fails under `enforce = true`. Only a cache miss falls back
this way: a cached include that violates its pin, or any other load error, is reported as the error it is.

### `lock --diff`

The same comparison, printed for a pull request description or a bot, and always exit code `0`. `--format json`
prints the document described by
[`schema/lock-diff.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/lock-diff.schema.json):

```json
{
  "schema_version": 1,
  "in_sync": false,
  "lock_version": 1,
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
enforce = true           # default whenever ai-rulez.lock exists; false opts out. Strict validation reports drift (and an unreadable lock), AR010 is an error, generate refuses an unlocked remote source, generate --locked requires content pins
include_outputs = true   # false: pin sources only
scope = "all"            # "skills": pin only skills, remote includes and installed skills
```

`scope = "skills"` is for projects that only care about the skill supply chain; it implies no output pins. The
lock records the settings it was written with, and `--check` reports a mismatch so a changed `[lock]` table cannot
silently weaken a check.

With `enforce = true`, and only when a lock exists, `validate --strict` adds:

| Code | Meaning |
| --- | --- |
| `AR981` `lock-source-drift` | An authored item was added, removed or changed since the lock was written, the lock has no content pins, or the lock cannot be read or compared (corrupt, another lock `version`, an unpinnable source). Enforcement never skips a check it cannot run. |
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
