# Corpus harness

Runs a built `ai-rulez` binary against real repositories and reports, per repository and per phase, `PASS`, `FAIL`
or `SKIP` with a one-line reason. It is the "corpus iteration" check: everything the unit tests cannot see because it
needs a repository somebody actually works in (legacy manifests, remote conventions, big skill trees, worktrees).

No network, no LLM calls, no secrets. The binary runs under `env -i` with a private `HOME`, a dead HTTP proxy, and
no credential variables. Keys are throwaway files made with `openssl`.

## Usage

```bash
CGO_ENABLED=0 go build -o /tmp/ai-rulez ./cmd/ai-rulez

# repositories from a file (one path per line, # comments) ...
tests/corpus/run.sh /tmp/ai-rulez my-repos.txt
# ... or from the environment (whitespace or newline separated)
CORPUS_REPOS="$HOME/code/a $HOME/code/b" tests/corpus/run.sh /tmp/ai-rulez

tests/corpus/run.sh -l                                   # list the phases
tests/corpus/run.sh -p signing,frozen /tmp/ai-rulez list # only these phases
tests/corpus/run.sh -x mcp_smoke -o out /tmp/ai-rulez list
```

There is deliberately no default repository list: pass your own. A path that is missing or not a git repository is
reported and skipped. Exit status: 0 all phases passed or skipped, 1 at least one `FAIL`, 3 a source repository
changed during the run, 64 usage error.

| Environment | Meaning |
| --- | --- |
| `CORPUS_REPOS` | repository paths when no list file is given |
| `CORPUS_TIMEOUT` | seconds allowed per `ai-rulez` invocation (default 300; a timeout fails the phase) |
| `CORPUS_ARCHIVE_EXCLUDE` | git pathspecs to leave out of the snapshot, for repositories with huge fixture trees |

## Output

`-o DIR` (default: a fresh directory under `$TMPDIR`) receives:

- `summary.json` with `schema_version` 1: the phases run, the state of every source (`untouched`/`MODIFIED`/`missing`),
  totals, per-phase counts and one entry per repository and phase (`status`, `seconds`, `reason`);
- `results.tsv`: the same rows as tab-separated text;
- `logs/<repo-id>/<phase>/NN.log` and `NN.log.cmd`: the output and command line of every invocation.

## Safety of the sources

Sources are strictly read-only. Each is copied once with `git archive HEAD` (with `GIT_OPTIONAL_LOCKS=0` and a
throwaway index file), turned into a one-commit repository of its own, and every phase works in a private
copy-on-write copy of that snapshot under `$TMPDIR/corpus-*`. The harness never runs a git write command in a source.
`git rev-parse HEAD` and `git status` of each source are compared before and after the run; a difference is reported
as `MODIFIED` and exit status 3.

## Phases

Every phase is an independent function `phase_<name>` (see `lib/phases_*.sh`) and starts from a fresh copy.

| Phase | What it checks |
| --- | --- |
| `upgrade` | `migrate v5` (dry run, real, idempotent) on a v4 config, then `generate` converges; legacy outputs listed in old manifests |
| `mcp_smoke` | the CRUD MCP server over stdio JSON-RPC: initialize, tools/list, rule create/read/update/delete, an unknown tool, a malformed line |
| `plugin` | plugin members: `generate --plugin`, `verify --plugin`, `publish --dry-run` (skipped without a `[plugin]`/`[marketplace]` block) |
| `skipped_content` | a copy with no config: no command panics or writes a file |
| `agent_conventions` | unreachable remote conventions degrade offline and fail closed under `--frozen`; a local git include is pinned by the lock and honoured by `--frozen` |
| `sparse` | a git sparse checkout without the content directories, and content without the config file |
| `local_includes` | a local include generates, tracks edits, and an include outside the project is refused |
| `profile` | every declared profile, `--check` for each, a composed pair, an unknown profile |
| `role` | every declared role: `roles show/resolve`, `generate --role`, `sbom --role` |
| `recursive` | `generate -r` and `validate -r` over a checkout plus a git worktree |
| `repaired` | a config that does not load is repaired with `migrate v5` and then validates and generates |
| `frozen` | `generate --frozen/--locked` keep the lock byte for byte; tampering with pinned content is caught |
| `signing` | key-based `sign --lock/--sbom`, `verify --attestation`, `approve` (asserted and signed), `verify --approvals`, wrong key and tamper negatives |
| `determinism` | `sbom` (both formats), `catalog` (json and html) and `export okf` are byte-identical across two runs and across two checkouts |
| `injected` | hooks, permissions, a role, verifiers and the machine-local overlay injected into the config |
| `offline_tools` | `eval run` (estimate and retrieval), `search`, `telemetry` without consent |
| `validate_formats` | text, json, sarif, github, junit and markdown reports; baselines accept old findings and still report new ones |
| `agent_plugins` | Agent Plugins package: generate, verify, `publish emit` twice, `convert --from agent-plugins` round trip |
| `ard` | `ard.json` from `publish emit ard`: byte-stable, one of url/data per entry, identifiers well formed |
| `llms_txt` | the `llms-txt` preset: generate, `validate --strict --analyzer llmstxt`, determinism, a broken file is rejected |

The newer-feature phases (`agent_plugins`, `ard`, `llms_txt`) build a small standalone project from the repository's own
rules, context and skills, so real content is exercised under a config that is valid on its own.

Phases that need to run without the network first migrate the copy to v5 and drop the config tables that fetch from
git or https (`[[includes]]`, `[[installed_skills]]`, `[[skill_sources]]`).

## Layout

```
run.sh                 entry point: arguments, snapshots, summary
lib/common.sh          results, the hermetic ai-rulez wrapper, the phase runner
lib/repo.sh            snapshots, config probes, prep helpers
lib/phases_config.sh   upgrade, repaired, skipped_content, agent_conventions, sparse, local_includes, profile, role, recursive
lib/phases_governance.sh   frozen, signing, injected
lib/phases_output.sh   mcp_smoke, plugin, determinism, validate_formats, offline_tools
lib/phases_features.sh agent_plugins, ard, llms_txt
repros/                minimal reproductions of product bugs the corpus found (each takes the binary path, exits 1 while the bug exists)
```

Add a phase by defining `phase_<name>` in a `lib/phases_*.sh` file, using `ar` to run the binary, `ok_note`/`fail_note`
to collect findings and `finish`, `pass`, `fail` or `skip` to end, and adding the name to the `ordered` list in `run.sh`.
Check scripts with `shellcheck tests/corpus/run.sh tests/corpus/lib/*.sh tests/corpus/repros/*.sh`.
