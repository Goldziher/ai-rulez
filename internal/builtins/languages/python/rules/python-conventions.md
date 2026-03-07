---
priority: high
---
# Python Conventions
- Target Python 3.10+. Use type hints for all public APIs.
- Follow PEP 8. Use `ruff` for linting and formatting.
- Use `pytest` for testing. Prefer fixtures over setup/teardown methods.
- Use `async`/`await` for IO-bound operations. Prefer `pathlib` over `os.path`.
- Use virtual environments. Pin dependencies with lock files.
