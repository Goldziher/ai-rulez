"""Offline release-version controls with independent, version-stable fixtures."""

import importlib.util
import json
import tempfile
import unittest
from pathlib import Path

SPEC = importlib.util.spec_from_file_location("set_version", Path(__file__).with_name("set_version.py"))
assert SPEC is not None and SPEC.loader is not None
set_version = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(set_version)

FIXTURES = {
    "cmd/commands/root.go": 'package commands\nconst (\n Version = "1.2.3"\n)\n',
    "cmd/ai-rulez/main.go": 'package main\nvar version = "1.2.3"\n',
    "release/npm/package.json": '{\n  "version": "1.2.3"\n}\n',
    "release/pypi/__init__.py": '__version__ = "1.2.3"\n',
    "release/pypi/ai_rulez/__init__.py": '__version__ = "1.2.3"\n',
    "internal/hooks/setup.go": 'package hooks\nconst (\n officialPreCommitRev = "v1.2.3"\n)\n',
    "scripts/pre-commit/run-ai-rulez.sh": 'AI_RULEZ_VERSION="${AI_RULEZ_VERSION:-v1.2.3}"\n',
    "README.md": 'rev: v1.2.3\nrevision = "v1.2.3"\n',
    "poly-hooks.toml": 'command = "npx ai-rulez@latest && uvx ai-rulez==latest"\n',
    "website/src/content/docs/poly-hooks.md": 'revision = "v1.2.3"\nnpx ai-rulez@latest\nuvx ai-rulez==latest\n',
    "website/src/content/docs/installation.md": "ai-rulez version 1.2.3\nrev: v1.2.3\nnpx ai-rulez@latest\nuvx --from ai-rulez==latest ai-rulez\n",
    "website/src/content/docs/github-actions.md": "- uses: Goldziher/ai-rulez@v5\n  with:\n    version: 1.2.3\n",
    "website/src/content/docs/approvals.md": "- uses: Goldziher/ai-rulez@v5\n  with:\n    version: 1.2.3\n",
    "website/src/content/docs/lockfile.md": "- uses: Goldziher/ai-rulez@v5\n  with:\n    version: 1.2.3\n",
}
PYTHON_METADATA = {"release/pypi/__init__.py", "release/pypi/ai_rulez/__init__.py"}


class VersionFixtures(unittest.TestCase):
    def setUp(self) -> None:
        self.assertEqual(len(FIXTURES), 14)
        self.assertEqual(sorted(FIXTURES), sorted({row[0] for row in set_version.RULES}))
        temporary = tempfile.TemporaryDirectory(prefix="ai-rulez-version-fixture-")
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        for relative, content in FIXTURES.items():
            path = self.root / relative
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(content)

    def snapshot(self) -> dict[str, bytes]:
        return {name: (self.root / name).read_bytes() for name in FIXTURES}

    def contents(self) -> dict[str, str]:
        return {name: (self.root / name).read_text() for name in FIXTURES}

    def test_stable_release_checks_all_surfaces_and_preserves_latest_channels(self) -> None:
        result = set_version.synchronize(self.root, "9.8.7")
        expected = {name: text.replace("1.2.3", "9.8.7") for name, text in FIXTURES.items()}
        self.assertEqual(result["files_checked"], 14)
        self.assertEqual(result["files_changed"], sorted(name for name in FIXTURES if expected[name] != FIXTURES[name]))
        self.assertEqual(sum(row["replacements"] for row in result["rules"] if row["required"]), 7)
        self.assertEqual(self.contents(), expected)
        self.assertEqual(json.loads((self.root / "release/npm/package.json").read_text())["version"], "9.8.7")

    def test_prerelease_uses_pep440_only_for_python_metadata(self) -> None:
        result = set_version.synchronize(self.root, "v9.8.7-rc.2")
        self.assertEqual(result["version"], "9.8.7-rc.2")
        expected = {
            name: text.replace("1.2.3", "9.8.7rc2" if name in PYTHON_METADATA else "9.8.7-rc.2")
            for name, text in FIXTURES.items()
        }
        self.assertEqual(self.contents(), expected)

    def test_second_sync_is_idempotent_but_checks_all_required_fields(self) -> None:
        set_version.synchronize(self.root, "9.8.7")
        before = self.snapshot()
        result = set_version.synchronize(self.root, "9.8.7")
        self.assertEqual(result["files_changed"], [])
        self.assertEqual(result["files_checked"], 14)
        self.assertEqual(before, self.snapshot())

    def test_missing_required_field_refuses_before_any_file_write(self) -> None:
        path = self.root / "scripts/pre-commit/run-ai-rulez.sh"
        path.write_text(path.read_text().replace("AI_RULEZ_VERSION", "OTHER_VERSION"))
        before = self.snapshot()
        with self.assertRaisesRegex(ValueError, "expected exactly one release field, found 0"):
            set_version.synchronize(self.root, "9.8.7")
        self.assertEqual(before, self.snapshot())

    def test_legacy_pinned_hook_examples_update_without_pinning_latest(self) -> None:
        for name in [
            "poly-hooks.toml",
            "website/src/content/docs/poly-hooks.md",
            "website/src/content/docs/installation.md",
        ]:
            path = self.root / name
            path.write_text(path.read_text() + "\n# npx ai-rulez@1.2.3; uvx ai-rulez==1.2.3\n")
        set_version.synchronize(self.root, "9.8.7-rc.2")
        for name in [
            "poly-hooks.toml",
            "website/src/content/docs/poly-hooks.md",
            "website/src/content/docs/installation.md",
        ]:
            text = (self.root / name).read_text()
            self.assertIn("ai-rulez@9.8.7-rc.2", text)
            self.assertIn("ai-rulez==9.8.7rc2", text)
            self.assertIn("ai-rulez@latest", text)
            self.assertIn("ai-rulez==latest", text)

    def test_action_pin_rule_ignores_other_version_keys(self) -> None:
        path = self.root / "website/src/content/docs/github-actions.md"
        other = "plugin:\n  version: 1.2.3\n- uses: Goldziher/ai-rulez/check@v5\n  with:\n    profile: ci\n    version: 1.2.3\n"
        path.write_text(path.read_text() + other)
        set_version.synchronize(self.root, "9.8.7")
        text = path.read_text()
        self.assertIn("plugin:\n  version: 1.2.3\n", text)
        self.assertIn("    profile: ci\n    version: 9.8.7\n", text)
        self.assertEqual(text.count("version: 9.8.7"), 2)

    def test_invalid_version_refuses_without_writes(self) -> None:
        before = self.snapshot()
        with self.assertRaisesRegex(ValueError, "version must be"):
            set_version.synchronize(self.root, "9.8.7; unwanted-command")
        self.assertEqual(before, self.snapshot())

    def test_floating_major_examples_and_latest_channels_are_preserved(self) -> None:
        for name in ["README.md", "website/src/content/docs/poly-hooks.md"]:
            path = self.root / name
            path.write_text(path.read_text().replace("v1.2.3", "v5"))
        path = self.root / "poly-hooks.toml"
        path.write_text(path.read_text() + "\n# npx -y ai-rulez@5\n")
        before = self.contents()
        result = set_version.synchronize(self.root, "9.8.7")
        for name in ["README.md", "website/src/content/docs/poly-hooks.md", "poly-hooks.toml"]:
            self.assertEqual(self.contents()[name], before[name])
        self.assertEqual(json.loads(self.contents()["release/npm/package.json"])["version"], "9.8.7")
        self.assertEqual(sum(row["replacements"] for row in result["rules"] if row["required"]), 7)


if __name__ == "__main__":
    suite = unittest.defaultTestLoader.loadTestsFromTestCase(VersionFixtures)
    assert suite.countTestCases() == 8
    result = unittest.TextTestRunner(verbosity=2).run(suite)
    assert result.testsRun == 8 and not result.skipped
    raise SystemExit(0 if result.wasSuccessful() else 1)
