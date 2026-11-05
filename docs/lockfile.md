# Lock file

`ai-rulez.lock` records exactly what your AI configuration is made of: every remote source by commit and digest, every
authored rule, skill, hook and role by content digest, and every generated file by output digest. Commit it, and CI
fails whenever any of it changes without a reviewed lock update.

## Example

```bash
ai-rulez generate
ai-rulez lock               # writes .ai-rulez/ai-rulez.lock
git add .ai-rulez/ai-rulez.lock
```

```toml
# .ai-rulez/ai-rulez.lock (excerpt)
version = 1
ai_rulez_version = '5.10.0'
scope = 'all'
outputs_pinned = true
tree = 'sha256:4dd87fd5ae23b632928921220800a83b679b58a58cd0e2fa3127cd5196dfcbee'

[[item]]
kind = 'hook'
id = 'PreToolUse:Bash:0'
digest = 'sha256:b99ff8c5372c10a70d8862804dbbbc53d26999e3a78f97ebfea20b4cd1312fd3'

[[item]]
kind = 'skill'
id = 'deploy'
path = 'skills/deploy'
digest = 'sha256:70094bee730997ae05d11499b29571a05dde6cb5f17a4af98ed29465ba50d855'
owner = 'platform'
version = '1.2.0'

[[output]]
path = '.claude/skills/deploy/scripts/run.sh'
digest = 'sha256:e1bc260327c43375d60e256fc3326b836570497802c043c40572acbc921831a4'

[[output]]
path = 'AGENTS.md'
digest = 'sha256:2f8b8a8663f9f37c61f43723c6588bcf0b0191d90a558f59bce03ea24727d3e9'
...
```

Someone then edits a skill script and adds a rule. The check names each difference and exits `2`:

```console
$ ai-rulez lock --check
ai-rulez.lock does not match /work/acme/.ai-rulez:
  ...
  source added    rule security (rules/security.md)
  source changed  skill deploy (skills/deploy)
  output changed  output .agents/skills/deploy/scripts/run.sh
  output changed  output .claude/skills/deploy/scripts/run.sh
  output changed  output AGENTS.md
run `ai-rulez lock` to refresh it (after reviewing the change with `ai-rulez lock --diff`)
```

Review the change, then accept it with `ai-rulez lock`. `generate` never writes the lock; accepting a change is always
an explicit `ai-rulez lock`.

## What is pinned

| `kind` | id | What the digest covers |
| --- | --- | --- |
| `rule`, `context`, `agent`, `command`, `check` | the item name | the source file; a command's resources; the project scripts that frontmatter `hooks` run (a word starting with `./`, `../` or `$CLAUDE_PROJECT_DIR/`) |
| `skill` | the skill directory name | `SKILL.md`, every loaded resource (`references/`, `scripts/`, `assets/`) and the project scripts its frontmatter `hooks` run |
| `hook` | `<event>:<matcher or *>:<n>` | the `[[hooks]]` group as declared and each `script` file |
| `role` | the role name | the `[[roles]]` entry as declared |
| `settings` | `permissions`, `claude-managed`, `mcp-servers`, `verifiers-settings` | the `[permissions]`, `[claude.settings.managed]`, `[[mcp_servers]]` (placeholders unresolved) and `[verifiers_settings]` tables |
| `verifier` | the verifier name | the declaration as written, in `config.toml` or `.ai-rulez/verifiers/*.toml`; lowering a `severity`, widening an `exclude` or deleting a verifier changes the pin |
| `rubric` | the rubric directory name | every file under `.ai-rulez/rubrics/<id>/`, including golden cases and `calibration.json` ([Review](review.md#rubrics)) |
| `local-include` | the include name | the content directories of an include whose `source` is a local path; an OKF include is pinned whole |
| `local-skill` | the skill name | the directory of an `[[installed_skills]]` entry with a local source |
| `local-marketplace`, `local-plugin` | the entry name | the whole tree of an `[[installed_marketplaces]]` or `[[installed_plugins]]` entry with a local source |

Remote sources have their own tables, pinned by commit and tree digest: `[[include]]`, `[[skill]]` (installed skills),
`[[source]]` (skill sources), `[[marketplace]]` and `[[plugin]]`
([Installed plugins](installed-plugins.md#lock-update-and-refresh)).
Built-in packs are pinned by the ai-rulez version.

**Outputs** (`[[output]]`) are pinned from the in-memory rendering with the `Content-Hash`, `Source-Hash` and
`Generated:` header lines removed, so the digests do not depend on `[header]` settings. Not pinned: machine-local
outputs, outputs that may carry resolved secrets, and merged documents in which you own some keys (their ai-rulez
sources are pinned instead).

**Not pinned at the source**: profiles, `include` configuration, scoped (monorepo) configuration, plugin and
marketplace authoring, and the machine-local overlay. A change there shows up only through the output pins, so keep
`include_outputs` on if you rely on the lock for them. A `local_override` path is a development shortcut and is not
pinned.

A hook script outside the project cannot be pinned: `lock --check` reports it until it moves inside the project. A
symlink inside a pinned tree is never followed; it is pinned by its target string. A symlinked `ai-rulez.lock` is
refused by every command that reads it, and `lock` replaces it with a regular file.

## File format

| Key or table | Meaning |
| --- | --- |
| `version` | Lock format: `1`, or `2` when the lock holds role output pins. Any other version is refused with a request to run `ai-rulez lock` |
| `ai_rulez_version` | The release that wrote the content pins. A different running version is a note, not a failure |
| `profile` | The profile the output pins were rendered for, when not the default |
| `scope`, `outputs_pinned` | The `[lock]` settings the pins were written with; `lock --check` reports a mismatch, so a changed `[lock]` table cannot silently weaken the check |
| `tree` | One digest over every pin and remote entry. It catches accidental edits and bad merges; it is not a signature (see [Signing the lock](#signing-the-lock)) |
| `[[include]]`, `[[skill]]`, `[[source]]`, `[[marketplace]]`, `[[plugin]]` | Remote sources: `name`, `source`, `path`, `ref`, `commit`, `digest`, and with a [version constraint](#version-constraints) `tag`, `tag_object`, `released`, `released_from` |
| `[[served]]` | Skills the MCP skills server serves ([Served skills](#served-skills-and-skill-sources)) |
| `[[item]]` | Authored content: `kind`, `id`, `domain`, `path`, `digest`, and `owner` and `version` from frontmatter |
| `[[output]]` | Generated files: `path` and `digest`, or `role` and `digest` for a [role pin](#composing-with-roles) |
| `[[approval]]`, `[[deny]]` | Reviewer approvals and denied digests ([Approvals](approvals.md)); outside `tree` |
| `[[scan]]` | External scanner results ([Scan records](#scan-records)); outside `tree` |

The file is deterministic: entries sorted, no timestamps, no machine paths. A skill that comes from an include is
recorded as `include:<name>/<path>`. Running `lock` twice produces the same bytes, and the digests are the same on
every operating system as long as git records the executable bit of your scripts (see [Hashing scheme](#hashing-scheme)).

### Scan records

`lock` writes one `[[scan]]` record per `egress = false` [external scanner](strict-validation.md#lock-records) that has
a cached result for the current content. `scan --external` fills the cache; `lock` starts no program.

```toml
[[scan]]
scanner = "cisco-skill-scanner"
version = "skill-scanner 1.2.0"
tree = "sha256:…"          # digest of the content staged for the scanner
findings = 2               # before the scanner baseline
max_severity = "warning"
result = "pass"            # fail when a finding reaches [lint.scanner_policy] fail_on
```

Records tell a reviewer what was scanned at lock time. They do not make `lock --check` fail, and a scanner with no
cached result for the content has no record.

## Commands

```bash
ai-rulez lock                 # pin remotes (network), content and outputs
ai-rulez lock --content-only  # re-pin authored content and outputs only; offline, remote pins kept
ai-rulez lock shared          # re-pin one source by name; other pins stay as they are
ai-rulez lock --kind skill    # re-pin one kind: include, skill, source, marketplace, plugin or served
ai-rulez lock --check         # verify everything offline; exit 2 on any difference
ai-rulez lock --diff          # what `ai-rulez lock` would change; always exit 0
ai-rulez lock --subject       # the digest a signature over the lock covers
ai-rulez lock --outdated      # sources whose constraint allows a newer tag (network)
ai-rulez update --dry-run     # what moving the range pins would change
ai-rulez generate --locked    # fail when a source or output differs from the lock
ai-rulez generate --frozen    # --locked without the network
```

`lock` pins outputs for `--profile`, else the profile recorded in the lock, else the configured default. `lock` runs
the security scan over every remote tree it pins to something new and refuses error findings unless you pass
`--accept-findings` after reviewing them. `ai-rulez skill update` is `lock --kind skill`.

| `lock` exit code | Meaning |
| --- | --- |
| `0` | written, or `--check`: in sync |
| `1` | the command could not run; `--check` with no `ai-rulez.lock` |
| `2` | `--check`: differences; `lock`: a refused pin (scan findings, a moved tag, a denied digest, a policy violation) |
| `3` | written, but served skills the security scan refuses were left unpinned; `--refuse-findings` makes this exit `2` and write nothing |

With `--recursive` the most severe code over all roots wins: `1`, then `2`, then `3`.

### `lock --check`

Offline. It renders the outputs in memory, then compares the remote pins with the local cache, every authored item
with its pin, and every output with its pin. `source` lines say an authored item changed; `output` lines say what
your assistants see changed. An output can change alone (a new ai-rulez release, a different include revision), which
is worth a look too.

It also fails (exit `2`) on a lock that pins no content, whatever `enforce` says, so stripping the content pins
cannot switch the check off. If remote includes are configured but not in the local cache, it compares sources only and
says so in a note, or fails under `enforce`.

### `lock --diff`

The same comparison for a pull request description or a bot, always exit `0`. `--format json` follows
[`schema/lock-diff.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/lock-diff.schema.json):

```json
{
  "schema_version": 1,
  "in_sync": false,
  "lock_version": 1,
  "lock_ai_rulez_version": "5.10.0",
  "ai_rulez_version": "5.10.0",
  "changes": [
    {
      "scope": "source",
      "change": "changed",
      "kind": "skill",
      "id": "deploy",
      "path": "skills/deploy",
      "old": "sha256:1741f834511e63e2a855dbcf0510ff822543f127b3735f786971099bc5ba87e6",
      "new": "sha256:015acba08ec153ad3737ab523d24b647112572b129555371a5dccc4b0f566359"
    },
    {
      "scope": "output",
      "change": "changed",
      "path": ".claude/skills/deploy/scripts/run.sh",
      "old": "sha256:e1bc2603…",
      "new": "sha256:b9bd9b58…"
    }
  ]
}
```

| `scope` | Meaning |
| --- | --- |
| `source` | an authored item was added, removed or changed |
| `output` | a generated file differs from its pin |
| `remote` | a remote source's cached tree no longer matches its pin |
| `served` | a served skill differs from its pin, or is no longer served |
| `lock` | the lock itself: hand-edited pins, or written with different `[lock]` settings |
| `approval` | content whose [approval](approvals.md) is missing, stale or insufficient |

`lock --check --format json` prints the same document and exits `2` on any difference.

## Configuration

```toml
[lock]
enforce = true
include_outputs = true
scope = "all"
min_release_age = "7d"
min_release_age_source = "auto"
verify_tags = false
refresh_on_generate = false
```

| Key | Type | Default | Meaning |
| --- | --- | --- | --- |
| `enforce` | bool | `true` when `ai-rulez.lock` exists | `validate` reports drift (`AR981`, `AR982`), `AR010` is an error so `generate` refuses an unpinned remote source, `lock --check` fails on a missing lock, `generate --locked` requires content pins, and the skills server refuses unpinned or changed skills. `false` opts out |
| `include_outputs` | bool | `true` | `false` pins sources only |
| `scope` | string | `"all"` | `"skills"` pins only skills, remote includes and installed skills (implies no output pins) |
| `min_release_age` | string | none | Default [minimum release age](#minimum-release-age) for sources with a version constraint |
| `min_release_age_source` | string | `"auto"` | Where a tag's release time comes from: `auto`, `forge`, `first-seen` or `commit` |
| `verify_tags` | bool | `false` | `generate` and `lock --check` [ask the remotes](#checking-pinned-tags-online) whether a pinned tag moved |
| `refresh_on_generate` | bool | `false` | `generate` [refreshes the lock](#refreshing-on-generate) before writing outputs |

With `enforce` on, `validate` adds:

| Code | Meaning |
| --- | --- |
| `AR981` `lock-source-drift` | An authored item was added, removed or changed since the lock was written; or the lock has no content pins, or cannot be read or compared |
| `AR982` `lock-output-drift` | A generated output differs from its pinned digest |

Both default to `error` and can be tuned in `[lint.severity]`.

## CI usage

```yaml
- run: ai-rulez lock --check          # pins match: sources, outputs, remote cache
- run: ai-rulez generate --check      # the committed output matches the sources
- run: ai-rulez generate --locked     # fail if a source changed without a lock update
```

`generate --check` asks "was the output regenerated after the sources changed?". `lock --check` asks "was the change
accepted?": a source edit, a new skill or a moved include must come with a lock update, which is a separate, small,
reviewable diff. Paste `ai-rulez lock --diff` into the pull request description.

### Scheduled outdated report

A scheduled job reports sources that fall behind. It reads tags only and writes nothing. It also records every tag it
sees in the machine's `first-seen` history (`~/.cache/ai-rulez/observed-tags.toml`), which a
[minimum release age](#minimum-release-age) can use. That history is local to the machine: on a hosted runner it is lost
after the job unless you cache it, and it never helps a `lock` or `update` that runs on another machine.

```yaml
name: ai-rulez outdated
on:
  schedule:
    - cron: "17 6 * * *"
  workflow_dispatch:
permissions:
  contents: read
jobs:
  outdated:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
      - uses: Goldziher/ai-rulez@v5
        with:
          version: 5.10.0
      - uses: actions/cache@v4                    # keep the first-seen history between runs
        with:
          path: ~/.cache/ai-rulez/observed-tags.toml
          key: ai-rulez-observed-tags-${{ github.run_id }}
          restore-keys: ai-rulez-observed-tags-
      - name: Report outdated sources
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}   # release dates; sent only to github.com
        run: ai-rulez lock --outdated --format json --fail-on-outdated | tee outdated.json
      - name: Verify pinned tags did not move
        run: ai-rulez lock --check --verify-tags
```

`--fail-on-outdated` turns the job red until someone runs `ai-rulez update` and merges the lock. To have CI open the
update pull request itself, use the [`update` sub-action](github-actions.md#scheduled-pin-updates-update). To report
updatable sources as findings instead, enable `AR734`:

```toml
[lint.severity]
AR734 = "warning"     # "error" also makes lock --outdated exit 2
```

## Reviewing lock diffs

One block per item, sorted by kind, domain and id, so `git diff ai-rulez.lock` reads like a change list:

- a new `[[item]]` is a new rule, skill, hook or role: open the file at `path`;
- a changed `digest` with an unchanged `version` means the content moved but nobody bumped the version;
- a changed `owner` is a change of responsibility;
- a new or changed `kind = "hook"` item, or a skill script, is code that runs on developers' machines: read it;
- `[[output]]` churn with no `[[item]]` change means the rendering changed (a release, an include): find out why.

The digest tells you *that* something changed, not what. Read it next to the source diff; a lock change with no
matching source change deserves a question.

## Version constraints

A remote include, installed skill or skill source can ask for a range of versions instead of a ref. ai-rulez resolves
the range against the repository's semantic-version tags, records the exact tag, its commit and the tree digest, and
never moves the pin on its own.

```toml
[[includes]]
name = "shared"
source = "https://github.com/acme/ai-rules"
version = "^1.2"
tag_prefix = "v"                 # optional; default accepts "1.2.3" and "v1.2.3"
include_prerelease = false       # optional
min_release_age = "7d"           # optional, see below

[[installed_skills]]
name = "deploy"
source = "https://github.com/acme/skills"
version = "~2.1.0"
tag_prefix = "deploy/v"          # monorepo tags such as deploy/v2.1.3

[[skill_sources]]
name = "vendor"
url = "https://github.com/acme/skills"
version = ">=1.4.0 <2.0.0"
```

The lock records the constraint as `ref` and what it resolved to:

```toml
[[include]]
name = 'shared'
source = 'https://github.com/acme/ai-rules'
ref = '^1.2'                          # the constraint, as written
tag = 'v1.2.4'                        # the resolved tag
tag_object = '7a9c…'                  # annotated tag object; absent for a lightweight tag
commit = '0f3e…'                      # the peeled commit
digest = 'sha256:…'                   # tree digest
released = '2026-09-28T10:00:00Z'     # only with min_release_age
released_from = 'forge'               # forge, first-seen or commit
```

| Topic | Behavior |
| --- | --- |
| Syntax | npm-style: `^1.2`, `~2.1.0`, `>=1.4.0 <2.0.0` (a space is AND), `1.x`, `1.2.*`, `1 - 2`, `1.x \|\| 3`, `*`. `^0.2.3` stays below `0.3.0` |
| `ref` shorthand | `ref = "^1.2"` means `version = "^1.2"` when the value contains `^`, `~`, `*` or a space (git forbids those in ref names). Setting both `ref` and `version` is `AR731` |
| Version tags | a plain version, one after a single `v`, or with `tag_prefix` the prefix plus a plain version. Annotated tags are peeled to their commit. The highest allowed tag wins |
| Prereleases | skipped unless the constraint names a prerelease of the same version (`^1.3.0-rc.1`) or `include_prerelease = true`. Build metadata is ignored |
| Who resolves | `lock` resolves a range the lock does not cover yet, and keeps a pin that still satisfies its constraint. `generate` uses the pinned commit; without a pin it resolves once, like a branch. `generate --locked`/`--frozen` fail on an unpinned range. Only `update` moves a range pin |
| Pin validity | a pin covers the config while `source`, `path` and `ref` match and the recorded tag still satisfies the constraint and `tag_prefix`. Edit the constraint and the pin is stale until `lock` |

`released` and `released_from` are informational: they are outside `tree` and `lock --check` never compares them.

### Minimum release age

A tag published a moment ago may be a compromised release. `min_release_age` makes `lock` and `update` take the newest
allowed tag that has existed for at least that long:

```toml
[lock]
min_release_age = "7d"             # default for every source with a version constraint
min_release_age_source = "auto"

[[includes]]
name = "shared"
source = "https://github.com/acme/ai-rules"
version = "^1.2"
min_release_age = "3d"             # per source; "0" turns the gate off for it
```

Ages are a whole number with `h`, `d` or `w` (`12h`, `7d`, `2w`), at most 3650 days. A tag held back is reported as
`AR733` (info) and the next older tag that passes is used. The pinned tag is exempt: the gate never rolls a pin back.
If every allowed tag is too young and the source is not pinned yet, the result is `AR730` with the date the newest tag
becomes eligible. `lock --check` does not re-evaluate ages.

The release time comes from:

| `min_release_age_source` | Release time | Notes |
| --- | --- | --- |
| `forge` | the publish time of the tag's GitHub release ([Forge client](forge.md)) | a committer cannot forge it. When the release names a commit and the tag now points elsewhere, the time does not count. When the release names a branch, the time is the later of the publish time and the commit's first-seen time (no first-seen record: held back). A tag without a release has no time |
| `first-seen` | when this machine first saw the tag at that commit (`~/.cache/ai-rulez/observed-tags.toml`) | a tag never seen before counts as released now, so it is held back. A tag that moves counts as new |
| `commit` | the tagger or committer date | anyone who can push can forge it; protects against accidental adoption only |
| `auto` (default) | `forge`, then `first-seen` | never falls back to `commit`; when the first-seen record cannot be read or written, it warns and holds the tag back |

Every source fails closed: a tag whose time cannot be found is held back. With `auto` and a non-GitHub source, a new
project cannot lock a range until the age passes, because every tag is "seen now". The first-seen history only helps
on the machine that recorded it: run `lock --outdated` regularly on the same machine that later runs `lock` or `update`
(a self-hosted runner, or a hosted runner that caches `~/.cache/ai-rulez/observed-tags.toml`), use `commit` if that is
acceptable for the source, or pin a commit SHA.

### `lock --outdated`

```console
$ ai-rulez lock --outdated
SOURCE  KIND     CONSTRAINT  LOCKED         ALLOWED        LATEST         NOTE
shared  include  ^1.2        v1.2.4         v1.3.1         v2.0.0         update available; newer major available
deploy  skill    ~2.1.0      deploy/v2.1.3  deploy/v2.1.3  deploy/v2.2.0  up to date within the constraint
2 source(s): 1 updatable, 1 with a newer major, 0 moved tag(s), 0 error(s)
```

It lists tags only (one `git ls-remote` per repository) and writes nothing. Limit it with source names or
`--kind` (`include`, `skill`, `source`, `marketplace` or `plugin`). With a minimum release age, `ALLOWED` is the newest tag that is old enough. `--format json`
follows [`schema/lock-outdated.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/lock-outdated.schema.json),
with a `status` per source: `up-to-date`, `updatable`, `not-locked`, `tag-moved` (`AR732`), `tag-missing` (`AR735`),
`unsatisfiable` (`AR730`), `invalid` (`AR731`), `downgrade-only` or `locked-non-version`.

Exit codes: `0`, also when updates exist; `2` with `--fail-on-outdated` when any source has an allowed update, and
always for a moved or deleted tag or an unsatisfiable constraint; `1` when it could not run. It needs the network:
`--offline` refuses. `lock --check` is the offline verification.

### `update`

```console
$ ai-rulez update --dry-run
include shared: v1.2.4 -> v1.3.1 (b21c0f3e1a9d)
  released 2026-10-20T08:00:00Z (forge)
  M  rules/security.md
  A  skills/deploy/scripts/run.sh
  tree sha256:... -> sha256:...
  scan: 0 findings
  run `ai-rulez generate`, then `ai-rulez lock` (it refreshes the output pins and the served-skill pins, which stay stale until then)
would update 1 source(s)

$ ai-rulez update shared          # rewrites the lock only
$ ai-rulez update --kind skill    # only installed skills
```

`update [name...]` moves range pins to the newest allowed tag and rewrites the lock; it edits `config.toml` only with
`--major --write-config`. Output pins are not recomputed: run `generate` and `lock` afterwards. `--format json` follows
[`schema/update.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/update.schema.json).

| Situation | Behavior | Override after review |
| --- | --- | --- |
| A pinned tag now points to another commit (`AR732`) | refused, exit `2`, nothing written; `lock` refuses it too | `--accept-moved-tag` |
| The pinned tag was deleted (`AR735`) | warning; the pinned commit is still used | |
| The only newer tag has lower precedence | never selected (a truncated tag list or a mirror could roll a pin back) | `--allow-downgrade` |
| No tag satisfies the constraint (`AR730`) | exit `2` | fix the constraint |
| A tag is younger than `min_release_age` (`AR733`) | the newest old-enough tag is chosen | |
| The new tree has error-level security findings | refused, exit `2`, nothing written | `--accept-findings` |

`--dry-run` fetches and scans the new trees and writes nothing. The MCP server exposes the same operations as the tools
`lock_outdated`, `update_plan`, `lock_diff` and `update`; overrides such as `--accept-findings` stay on the command line
([Pin tools](mcp-server.md#pin-tools-lock_outdated-update_plan-update)).

#### Taking a new major version

```console
$ ai-rulez update --major
include shared: newer major v2.0.0: version = "^2.0" (now "^1.2"); `update --major --write-config` applies it
$ ai-rulez update --major --write-config shared
```

`--write-config` rewrites only the `version` value of that entry in `config.toml` (comments and layout stay), then
moves the pin with the same refusals as a plain `update`. If anything is refused, `config.toml` is restored byte for
byte. The entry must be a `[[includes]]`, `[[installed_skills]]` or `[[skill_sources]]` table with a `name` in the
project's own `config.toml`; anything else is refused with a hint to edit by hand.

### Checking pinned tags online

`lock --check` and `generate` stay offline. To ask the remotes whether the pinned tags moved, use
`lock --check --verify-tags`, `generate --verify-tags`, or `[lock] verify_tags = true`. One `git ls-remote` per
repository compares each locked tag with its pinned commit: a moved tag is `AR732` (exit `2`, `generate` writes
nothing), a deleted one `AR735` (warning). An unreachable remote is exit `1`. The config key is skipped under
`--offline` and `--frozen`; the flag with them is an error.

### Refreshing on generate

`generate --refresh-remote` (or `[lock] refresh_on_generate = true`) runs a lock refresh before the outputs are
written: every remote source moves to the newest version its constraint allows (a plain `ref` follows its tip),
`ai-rulez.lock` is rewritten, and generation continues from the new pins. All the defenses of `lock` apply: the
security scan, moved-tag and downgrade refusals, and `min_release_age`. Any refusal ends the run before an output is
written and leaves the lock unchanged; after review, `generate --refresh-remote --accept-findings` pins the trees anyway.

It needs the network and cannot be combined with `--check`, `--dry-run`, `--emit-plan`, `--locked`, `--frozen`,
`--offline`, `--watch`, `--recursive`, `--user` or `--plugin`; the config key is skipped in those runs.

## Served skills and skill sources

The same lock pins [dynamic skill loading](mcp-server.md#dynamic-skill-loading), so one `lock` pins everything and one
`lock --check` verifies everything:

```toml
[[source]]            # one per [[skill_sources]] entry
name = 'vendor'
source = 'https://github.com/acme/skills'
ref = 'v1.2.0'
path = 'skills'
commit = '0f3e…'
digest = 'sha256:…'

[[served]]            # one per skill the skills server serves
name = 'deploy'
source = '.ai-rulez/skills/deploy/SKILL.md'
commit = ''
digest = 'sha256:a7a18f7b364fef319e5cdab7388daa7d0a63d3e302493d44eca4811c09427304'

[[served]]            # the same skill in another view
name = 'deploy'
view = 'role:backend'
source = '.ai-rulez/skills/deploy/SKILL.md'
commit = ''
digest = 'sha256:…'
```

A served entry is keyed by name and `view`, the way the skills server is started: `role:<name>` for `--role`,
`profile:<name>`, `targets:<preset>`, `static` for `--include-static`, and `source:<name>` per `--source`, joined by `+`
(`role:backend+static`). The default view has no `view` key. A plain `lock` pins the default view, every role and every
view the lock already records; `lock --role`, `--profile`, `--targets`, `--include-static` and `--source` add a view.

The served digest covers the files the server returns, with the generated header lines that change without the skill
changing left out. Served files are digested at mode `100644`: the executable bit is not served over MCP.

| Command | Effect on served pins |
| --- | --- |
| `lock` | a skill the security scan refuses is left unpinned and `lock` exits `3`; `--refuse-findings` writes nothing and exits `2` |
| `lock --check`, `lock --diff` | a changed served skill is a `served` change, a source whose cached tree no longer matches is a `remote` change; no network |
| `lock --kind source` / `--kind served` | refreshes one kind |
| `lock --content-only` | recomputes authored content and the served digests of local skills offline |
| `mcp --serve-skills` | with a lock, refuses unpinned or changed skills (`AR995`); `--frozen` never uses the network |

## Composing with roles

[Roles](roles.md) are pinned as items (`kind = "role"`), so a new role or a changed `include`/`exclude` list is a lock
change. A role's rendered outputs are pinned only on request, as one aggregate digest per role:

```toml
[[output]]
role = 'backend'
digest = 'sha256:…'
```

A role is pinned when it declares `pin = true`, with `lock --roles` (every role) or `lock --role <name>`. Pins from the
command line carry `requested = true` and stay pinned on a plain `lock`. Removing `pin = true` unpins a role that only
its own key pinned. A lock with role pins is `version = 2`; an older ai-rulez refuses it rather than misreading it.

- The digest covers the files `generate --role <name>` would write, including the ai-rulez part of
  `.claude/settings.json`. The role is rendered in memory; nothing is written.
- `lock --check` compares every pinned role (`output changed  outputs of role backend`), including changes that leave
  the sources alone, such as a new generator release. `--role <name>` limits the comparison to one role.
- `lock --diff` adds the per-file digests of a changed role's current rendering (`files` in the JSON).
- `generate --locked --role <name>` verifies every source and, for a pinned role, its rendered outputs.

`ai-rulez catalog` reports, per item, its digest and whether the sources are in sync with the lock.

## Signing the lock

The lock proves that bytes did not change; a signature says who vouched for them. `ai-rulez sign --lock` and
`ai-rulez verify --attestation` do this with a `[signing]` policy ([Signing](signing.md)). You can also sign with
`cosign` directly, as below; `verify --attestation` accepts the bundle it writes.

What is signed is the **lock subject**, a digest over the meaning of the lock rather than its bytes, so line endings and
key order do not matter, while a hand-edited pin changes the subject:

```text
lock_subject = SHA256( lp("ai-rulez/lock-subject/v1") || lp(tree) || lp(approvals_digest)
                       || u64(hash_version) || lp(scope) || u8(outputs_pinned) )
```

- `tree` is recomputed from the lock entries, never read from the file. `lock --subject` fails (exit `2`) when the
  stored `tree` disagrees, so a hand-edited lock is never signed.
- `approvals_digest` is `sha256( lp("ai-rulez/approvals/v1") || records )` over every `[[approval]]` and `[[deny]]`
  record in lock order, each field length-prefixed (kind, domain, id, digest, reviewer, assurance, `approved_at`,
  `expires`, `note`, accepted findings, `ref`, `attestation`; then the deny digest and reason). It is the empty string
  when the lock holds neither. Adding or changing an approval changes the subject, so sign after the last `approve`.
- `hash_version` is `1`; `scope` is the recorded `[lock] scope` (`all` when none); `outputs_pinned` is `1` or `0`.
- `lp`, `u64` are defined in [Hashing scheme](#hashing-scheme); `u8` is one byte.

```console
$ ai-rulez lock --subject
sha256:99d64ff310d90f8f15b52414748bf8a43f56d3f5abcdbd8750fcd01f459a4464  (lock-subject/v1; tree sha256:4dd87fd5ae23b632928921220800a83b679b58a58cd0e2fa3127cd5196dfcbee, approvals none, hash_version 1, scope all, outputs_pinned true)

$ ai-rulez lock --subject --output lock-subject.json
sha256:99d64ff310d90f8f15b52414748bf8a43f56d3f5abcdbd8750fcd01f459a4464  (lock-subject/v1; tree sha256:4dd87fd5…, approvals none, hash_version 1, scope all, outputs_pinned true)
wrote lock-subject.json
```

The statement (`schema/lock-subject.schema.json`) is deterministic, so the file a verifier recomputes is byte-identical
to the one that was signed:

```json
{
  "schema_version": 1,
  "type": "ai-rulez/lock-subject/v1",
  "subject": "sha256:99d64ff310d90f8f15b52414748bf8a43f56d3f5abcdbd8750fcd01f459a4464",
  "tree": "sha256:4dd87fd5ae23b632928921220800a83b679b58a58cd0e2fa3127cd5196dfcbee",
  "approvals_digest": "",
  "hash_version": 1,
  "scope": "all",
  "outputs_pinned": true
}
```

### Sign in the release workflow

Keyless with cosign from GitHub Actions (the signer is the workflow identity):

```yaml
permissions: { id-token: write, contents: read }
steps:
  - run: ai-rulez lock --check
  - run: ai-rulez lock --subject --output lock-subject.json
  - run: cosign sign-blob --yes --bundle .ai-rulez/ai-rulez.lock.sigstore.json lock-subject.json
```

Commit `.ai-rulez/ai-rulez.lock.sigstore.json` next to the lock. With a key: `cosign sign-blob --key cosign.key
--bundle ... lock-subject.json`. `ai-rulez sign --lock` writes the same file name (a DSSE attestation instead of a blob
signature); both verify with `ai-rulez verify --attestation`. Use one kind per release.

### Verify

Recompute the statement from the committed lock and tell cosign who may sign:

```bash
ai-rulez lock --check                         # the lock still matches the sources
ai-rulez lock --subject --output lock-subject.json
cosign verify-blob \
  --bundle .ai-rulez/ai-rulez.lock.sigstore.json \
  --certificate-identity "https://github.com/acme/ai-config/.github/workflows/release.yml@refs/heads/main" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  lock-subject.json
```

Verification fails when the lock changed after signing, a pin was edited, or the bundle belongs to another
repository's lock. `lock --check` is still needed: it compares the lock with the sources, while the signature only says
who vouched for the lock. The plain cosign recipe has no freshness or rollback check; `ai-rulez verify --attestation`
adds an identity policy, `max_age` and rollback detection on top of the same bundle. Plugin bundles and served skills
are signed separately ([Bundles, skills and SBOMs](signing.md#bundles-skills-and-sboms)).

## Hashing scheme

Use this section to check a digest with an independent implementation. All digests are SHA-256, written
`sha256:<64 hex digits>`. The scheme is part of the lock format: a change to it would change the lock `version`.

Notation: `lp(x)` is the 8-byte big-endian length of `x` followed by `x`; `u64(n)` is `n` as 8 bytes big-endian.

**File leaf**, one file of an item:

```text
leaf = SHA256( lp("ai-rulez/file/v1") || lp(path) || lp(mode) || lp(data) )
```

- `path` is relative to the item, `/`-separated, with no `.`, `..`, empty segment or backslash.
- `mode` is `100755` when the file is executable, else `100644`. On Linux and macOS the owner execute bit decides; on
  Windows the mode comes from the git index (`git ls-files -s`), else `100644`. Commit the executable bit
  (`git update-index --chmod=+x`) so every operating system pins the same digest.
- `data` is the raw bytes on disk. For documents and data files (`.md .markdown .mdc .mdx .txt .toml .yaml .yml .json
  .jsonc`) `CRLF` becomes `LF` first, so a checkout with `autocrlf` pins the same digest. Scripts (`.sh .bash .zsh .py
  .js .mjs .cjs .ts`) and every other file are hashed byte for byte, because `#!/bin/sh\r` fails to run: keep scripts
  `LF` in git (`*.sh text eol=lf` in `.gitattributes`).

**Item tree**, domain-separated per kind (`rule`, `context`, `skill`, `agent`, `command`, `check`, `hook`, `role`,
`verifier`, `rubric`, `settings`, `output`, `include`, `okf-include`, `installed-skill`, `skill-source`, `served-skill`):

```text
digest = SHA256( lp("ai-rulez/<kind>/v1") || u64(n) || leaf_1 || … || leaf_n )
```

The `n` leaves (32 bytes each) are sorted by `path`, bytewise; duplicate paths are an error. A single-file item is a
tree of one leaf named after the file (`SKILL.md`, `style.md`). A declared item (a hook, a role, a settings table) is a
leaf holding its compact JSON encoding, plus one leaf per hook script.

A remote tree (include, OKF include, installed skill, skill source) is the item tree over its regular files, with `.git`
and the root `.cache_meta.json` left out. A symlink is the leaf `SHA256(lp("ai-rulez/symlink/v1") || lp(path) ||
lp(target))` and is never followed; any other non-regular entry is `SHA256(lp("ai-rulez/irregular/v1") || lp(path))`.

**Top-level `tree`**:

```text
tree = SHA256( lp("ai-rulez/tree/v1") || u64(n) || { lp(kind) || lp(key) || lp(digest) }… )
```

over every pin, sorted by `(kind, key)`: `item/<kind>` with key `<domain> NUL <id>`; `output` with the path (role
pins: `output-role` with the role name); `include`, `installed-skill`, `skill-source` and `served-skill` with the name
and `<commit> <digest>`. `digest` is the `sha256:<hex>` text of the pin. `tree` also covers the `source`, `ref`, `path`,
`tag` and `tag_object` of remote entries and the `view` of served entries. Two items with the same kind, domain and id
get a `#2` suffix on the second, in path order.

### Test vectors

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
| directory, kind `include`: `rules/a.md` = `# A\n` (`100644`), `hooks/x.sh` = `#!/bin/sh\n` (`100755`) | `sha256:94a2c6de5e10aa7eef64adb55330c4a84c02d49566d56adcedbd7f5db2ddf01c` |
| the same directory, kind `okf-include` | `sha256:990b39514f6589c5e61d584b7f59173cb8b1a3bd260ee93af92ebedc819da5e7` |
| top digest of no entries | `sha256:e8c93a22e1ed47e16dd881a55dc4fbc5ba685af20083b93469265da901784029` |

## What the lock does not do

It does not say who published a change, sandbox anything, or tell a malicious edit from a good one. It makes every
change explicit and reviewable; a human still reviews it. Add [approvals](approvals.md) to record who reviewed,
[signing](signing.md) to prove who vouched for the lock, and the [security checks](strict-validation.md#security-checks)
for the content itself.

## Troubleshooting

| Symptom | Cause | Fix |
| --- | --- | --- |
| `no ai-rulez.lock in …: nothing to check` (exit `1`) | no lock yet | `ai-rulez lock` |
| `ai-rulez.lock does not exist but [lock] enforce = true` (exit `2`) | `enforce = true` and no lock | `ai-rulez lock` |
| `unsupported lock version N` | the lock was written by a newer or much older ai-rulez | upgrade ai-rulez, or run `ai-rulez lock` with the version your team uses |
| `lock --check` reports `output changed` after an upgrade, with no source change | a new ai-rulez renders differently | review the output diff, then `ai-rulez lock` |
| Digests differ between Windows and Linux | a script is executable on disk but not in git, or a script has CRLF line endings | `git update-index --chmod=+x <file>`; `*.sh text eol=lf` |
| `lock` exits `3` | a served skill fails the security scan and was left unpinned; the skills server will refuse it | fix the finding the warning names, then `ai-rulez lock` |
| `generate --locked` fails with `authored content differs from ai-rulez.lock` | a source changed without a lock update | `ai-rulez lock --diff`, review, `ai-rulez lock` |
| A new range source cannot be locked: every tag held back | `min_release_age` with `auto` and no release history | wait; build first-seen history by running `lock --outdated` regularly on the machine that runs `lock`; or use `min_release_age_source = "commit"` for that source |
