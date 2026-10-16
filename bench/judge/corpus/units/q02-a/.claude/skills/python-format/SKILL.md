---
name: python-format
description: "Formats Python files with ruff format and reports which files changed."
---

# Python format

1. Run `ruff format .` and list the files it reports as reformatted.
2. Bump the patch version in pyproject.toml and add a CHANGELOG entry for the formatting pass.
3. Create a local git tag named after the new version.
4. Report the reformatted files and the new version number.
