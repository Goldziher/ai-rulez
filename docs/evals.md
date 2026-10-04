# Evals

ai-rulez does not run evaluations and does not implement graders. It gives eval cases a supported place to live,
keeps them out of the context your agents load, ships them in plugin bundles when you ask, and can report skills
that have none. Running the cases is the job of whatever evaluation tooling your harness or team uses.

## Layout

```text
.ai-rulez/
  skills/
    deploy-staging/
      SKILL.md
      evals/                 # cases for this skill
        trigger-basic/
          case.yaml
          prompt.md
        graders/
          contains_release_note.py
  evals/                     # optional: cases that span skills, or cases filed per skill name
    deploy-staging/
      cross-skill-handoff.yaml
    README.md
```

- `skills/<name>/evals/` is a recognized directory. `generate` does not warn about it, and it is **never** written
  into any per-tool skill tree (`.claude/skills`, `.agents/skills`, ...): eval cases must not cost context.
- `.ai-rulez/evals/` is an optional project-level tree. Use it for cases that involve several skills, or when you
  prefer to keep cases away from the skill directory. `.ai-rulez/evals/<skill-name>/` counts as that skill's cases
  for the lint rule below.
- The file layout inside those directories (`case.yaml`, `prompt.md`, graders, JSON cases) is yours; ai-rulez
  copies the files byte for byte and does not parse them.

## Bundling cases into a plugin

Eval cases are authoring material, so a plugin bundle leaves them out by default. (Before this option existed a
skill's `evals/` directory was copied along with the rest of the skill directory; it is now excluded unless you opt
in.) Opt in with `include_evals`:

```toml
[plugin]
name = "billing"
version = "1.0.0"
include_evals = true
```

With it on, every runtime bundle that carries skills contains:

- `skills/<name>/evals/...` for each bundled skill, and
- `evals/...`, a copy of `.ai-rulez/evals/` (or `<content_root>/evals/` when `content_root` is set), at the bundle
  root next to `skills/`.

For a per-domain plugin (`[marketplace.from_domains]`) only `evals/<skill-name>/...` of the skills in that plugin is
bundled, so one plugin never carries another plugin's cases. The copy obeys the same rules as other bundled files:
regular files only, and a symlink that resolves outside the project is refused.

The provenance sidecar `.ai-rulez-generated.json` records a hash of every bundled file, cases included, so
`ai-rulez verify --plugin` fails when a case is edited or removed in the bundle.

## Linting for skills without cases

`evals-missing` (`AR962`) is part of [strict validation](strict-validation.md) and is **off by default**:

```toml
[lint.evals]
require = true                     # turns AR962 on at warning severity
allow = ["internal-*", "scratch"]  # skill names or globs exempt from the check

[lint.severity]
evals-missing = "error"            # or set the severity directly; this wins over require
```

A skill passes when `skills/<name>/evals/` or `.ai-rulez/evals/<name>/` contains at least one non-hidden file.
`.gitkeep` does not count.

```console
$ ai-rulez validate --strict
.ai-rulez/skills/deploy-staging/SKILL.md:2  warning  AR962 evals-missing  skill "deploy-staging" has no eval cases ...
```

## CI recipe

```bash
set -euo pipefail

ai-rulez validate --strict                 # exit 2 on findings at or above fail_on (AR962 included when enabled)
ai-rulez generate --plugin                 # writes the bundle, cases included with include_evals = true
ai-rulez verify --plugin                   # exit non-zero if any bundled file differs from its recorded hash

# Run your harness's evaluation tooling against the bundle. ai-rulez does not ship one:
# point it at the bundle directory that generate --plugin printed.
"$EVAL_COMMAND" "$BUNDLE_DIR"
```

Exit codes to gate on: `validate --strict` exits `0` clean, `1` for an invalid configuration and `2` for findings at
or above `fail_on`; `verify --plugin` exits non-zero on any mismatch; your evaluation command's own exit code decides
whether the cases passed.
