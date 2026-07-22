---
name: python-conventions
description: Python code conventions covering type hints, Ruff formatting/linting, mypy/pyright, pytest, async I/O, uv packaging, and dependency security scanning. Load when writing or reviewing Python code.
---

- Python 3.10+, type hints on all public APIs, avoid `Any` — use precise types, generics, or `typing.Protocol`.
- Formatting/linting: a fast linter/formatter (e.g., Ruff), zero warnings; strict static type checking (e.g., mypy or pyright). Security: a SAST tool (e.g., Bandit).
- Testing: `pytest` with function-based tests, `pytest-cov` (80%+), `hypothesis` for property-based.
- Error handling: specific exceptions only, never bare `except:`, `contextlib.suppress` for intentional ignoring.
- Dataclasses or Pydantic for structured data — avoid raw dicts for known schemas.
- `pathlib.Path` over `os.path` for filesystem operations. Google-style docstrings on public APIs.
- Async: `async`/`await` for I/O, never mix blocking and async, `asyncio.gather()` for concurrency.
- Package management: a fast, lockfile-based package manager (e.g., uv) with the lockfile committed, build with a PEP 517 backend (e.g., maturin or hatchling).
- Security: `pip-audit` for dependency CVE scanning. Zero tolerance for critical/high vulnerabilities.
- Logging: structured logging (key=value / JSON) — never f-strings in log calls.
- Pattern matching (`match`/`case`) for multi-branch type dispatch (3.10+).
- Anti-patterns: mutable default args, `import *`, global state, `time.sleep` in async.
