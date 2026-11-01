"""Synchronize the release version without platform-specific sed -i behavior."""

import argparse
import json
import re
import sys
from pathlib import Path

PINNED_RELEASE = r"[0-9]+\.[0-9]+\.[0-9]+[0-9A-Za-z.+-]*"

RULES = (
    ("cmd/commands/root.go", r'(?m)^(\s*Version\s*=\s*")[^"]+(")', "semver", True),
    ("cmd/ai-rulez/main.go", r'(?m)^(var version = ")[^"]+(")', "semver", True),
    ("release/npm/package.json", r'(?m)^(\s*"version": ")[^"]+(")', "semver", True),
    ("pyproject.toml", r'(?m)^(version = ")[^"]+(")', "python", True),
    ("release/pypi/__init__.py", r'(?m)^(__version__ = ")[^"]+(")', "python", True),
    ("release/pypi/ai_rulez/__init__.py", r'(?m)^(__version__ = ")[^"]+(")', "python", True),
    ("internal/hooks/setup.go", r'(?m)^(\s*officialPreCommitRev\s*=\s*"v)[^"]+(")', "semver", True),
    ("scripts/pre-commit/run-ai-rulez.sh", r'(AI_RULEZ_VERSION="\$\{AI_RULEZ_VERSION:-v)[^}]+(\}")', "semver", True),
    ("README.md", rf"(rev: v){PINNED_RELEASE}()", "semver", False),
    ("README.md", rf'(revision = "v){PINNED_RELEASE}(")', "semver", False),
    ("poly-hooks.toml", rf"(ai-rulez@){PINNED_RELEASE}()", "semver", False),
    ("poly-hooks.toml", rf"(ai-rulez==){PINNED_RELEASE}()", "python", False),
    ("docs/poly-hooks.md", rf"(ai-rulez@){PINNED_RELEASE}()", "semver", False),
    ("docs/poly-hooks.md", rf"(ai-rulez==){PINNED_RELEASE}()", "python", False),
    ("docs/poly-hooks.md", rf'(revision = "v){PINNED_RELEASE}(")', "semver", False),
)


def synchronize(root: Path, version: str) -> dict[str, object]:
    version = version.removeprefix("v")
    if re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+(?:-rc\.[0-9]+)?", version) is None:
        raise ValueError("version must be X.Y.Z or X.Y.Z-rc.N")
    paths = sorted({row[0] for row in RULES})
    original = {path: (root / path).read_text() for path in paths}
    updated = original.copy()
    counts = []
    for path, pattern, kind, required in RULES:
        target = version.replace("-rc.", "rc") if kind == "python" else version
        updated[path], count = re.subn(
            pattern,
            lambda match, target=target: match[1] + target + match[2],
            updated[path],
        )
        if required and count != 1:
            raise ValueError(f"{path}: expected exactly one release field, found {count}")
        counts.append({"path": path, "replacements": count, "required": required})
    changed = [path for path in paths if updated[path] != original[path]]
    for path in changed:
        (root / path).write_text(updated[path])
    return {"version": version, "files_checked": len(paths), "files_changed": changed, "rules": counts}


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("version")
    args = parser.parse_args()
    result = synchronize(Path(__file__).resolve().parents[2], args.version)
    sys.stdout.write(json.dumps(result, indent=2) + "\n")


if __name__ == "__main__":
    main()
