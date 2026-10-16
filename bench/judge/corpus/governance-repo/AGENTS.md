# Acme agent instructions

These conventions apply to every AI coding agent working in this repository.

## Python

- Packages MUST NOT contain `__init__.py` files; the build system does not need them.
- TODO comments MUST have the form `TODO(ACME-123): description`.

## Artifacts

- Skills, commands and agents MUST live in the `.claude/` directory of the narrowest directory that uses them.
- Agents SHOULD be given the smallest set of tools that does the job.
