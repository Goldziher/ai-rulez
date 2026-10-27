"""Offline release/ref API fixtures for the mutable Action alias."""

import importlib.util
import unittest
from pathlib import Path
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location("update_action_tag", Path(__file__).with_name("update-action-tag.py"))
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)
SHA = "a" * 40
PREVIOUS = "b" * 40
ANNOTATION = "c" * 40
BASE = "repos/owner/repo"


def release(tag="v5.4.0", **overrides):
    return {"tag_name": tag, "draft": False, "prerelease": False, **overrides}


class ActionTagTests(unittest.TestCase):
    def setUp(self):
        self.responses = {
            f"{BASE}/releases/tags/v5.4.0": release(),
            f"{BASE}/releases?per_page=100": [release()],
            f"{BASE}/git/ref/tags/v5.4.0": {"object": {"type": "tag", "sha": ANNOTATION}},
            f"{BASE}/git/tags/{ANNOTATION}": {"object": {"type": "commit", "sha": SHA}},
            f"{BASE}/git/matching-refs/tags/v5": [],
            f"{BASE}/compare/{PREVIOUS}...{SHA}": {"status": "ahead"},
        }
        self.writes = []

    def api(self, endpoint, method="GET", payload=None, paginate=False):
        if method == "GET":
            return self.responses[endpoint]
        self.writes.append((method, endpoint, payload))
        return {}

    def git(self, args):
        self.writes.append(("git", *args))

    def run_update(self, tag="v5.4.0"):
        return MODULE.advance("owner/repo", tag, SHA, api=self.api, git=self.git)

    def existing_ref(self, sha=PREVIOUS):
        self.responses[f"{BASE}/git/matching-refs/tags/v5"] = [
            {"ref": "refs/tags/v5", "object": {"type": "commit", "sha": sha}},
        ]

    def pushed(self, alias="v5", tag="v5.4.0", lease=""):
        return [
            ("git", "tag", "--annotate", "--force", "--message", f"{alias}: {tag}", alias, SHA),
            (
                "git",
                "push",
                f"--force-with-lease=refs/tags/{alias}:{lease}",
                "origin",
                f"refs/tags/{alias}:refs/tags/{alias}",
            ),
        ]

    def test_create_alias_peels_annotated_release_and_leases_absence(self):
        self.assertIn("Created v5", self.run_update())
        self.assertEqual(self.writes, self.pushed(lease=""))

    def test_advance_alias_leases_the_previous_ref_value(self):
        self.existing_ref()
        self.assertIn("Advanced v5", self.run_update())
        self.assertEqual(self.writes, self.pushed(lease=PREVIOUS))

    def test_lease_is_the_raw_tag_object_of_an_annotated_alias(self):
        previous_annotation = "d" * 40
        self.existing_ref()
        self.responses[f"{BASE}/git/matching-refs/tags/v5"][0]["object"] = {"type": "tag", "sha": previous_annotation}
        self.responses[f"{BASE}/git/tags/{previous_annotation}"] = {"object": {"type": "commit", "sha": PREVIOUS}}
        self.run_update()
        self.assertEqual(self.writes, self.pushed(lease=previous_annotation))

    def test_major_alias_is_derived_from_the_release(self):
        tag = "v6.1.0"
        self.responses.update(
            {
                f"{BASE}/releases/tags/{tag}": release(tag),
                f"{BASE}/releases?per_page=100": [release(tag), release("v5.9.0")],
                f"{BASE}/git/ref/tags/{tag}": {"object": {"type": "commit", "sha": SHA}},
                f"{BASE}/git/matching-refs/tags/v6": [],
            }
        )
        self.assertIn("Created v6", self.run_update(tag))
        self.assertEqual(self.writes, self.pushed(alias="v6", tag=tag))

    def test_dry_run_checks_without_creating_or_updating(self):
        for existing in (False, True):
            with self.subTest(existing=existing):
                if existing:
                    self.existing_ref()
                result = MODULE.advance("owner/repo", "v5.4.0", SHA, api=self.api, git=self.git, dry_run=True)
                self.assertIn("Would", result)
                self.assertEqual(self.writes, [])

    def test_rerun_is_noop_and_peels_existing_annotation(self):
        self.existing_ref()
        self.responses[f"{BASE}/git/matching-refs/tags/v5"][0]["object"] = {"type": "tag", "sha": ANNOTATION}
        self.assertIn("already points", self.run_update())
        self.assertEqual(self.writes, [])

    def test_older_release_never_rolls_back_newer_release(self):
        self.responses[f"{BASE}/releases?per_page=100"] += [release("v5.10.0")]
        self.assertIn("newer stable release", self.run_update())
        self.assertEqual(self.writes, [])

    def test_newer_commit_never_rolls_back_even_without_release_record(self):
        self.existing_ref()
        self.responses[f"{BASE}/compare/{PREVIOUS}...{SHA}"] = {"status": "behind"}
        self.assertIn("newer commit", self.run_update())
        self.assertEqual(self.writes, [])

    def test_divergent_history_fails_closed(self):
        self.existing_ref()
        self.responses[f"{BASE}/compare/{PREVIOUS}...{SHA}"] = {"status": "diverged"}
        with self.assertRaisesRegex(ValueError, "divergent"):
            self.run_update()
        self.assertEqual(self.writes, [])

    def test_release_must_be_public_and_stable(self):
        for field in ("draft", "prerelease"):
            with self.subTest(field=field):
                self.responses[f"{BASE}/releases/tags/v5.4.0"] = release(**{field: True})
                with self.assertRaisesRegex(ValueError, "published stable"):
                    self.run_update()
                self.assertEqual(self.writes, [])

    def test_prerelease_and_floating_refs_do_not_call_api(self):
        for tag in ("v5.5.0-rc.1", "v5", "v5.04.0", "5.4.0"):
            with self.subTest(tag=tag):
                self.assertIn("only stable", self.run_update(tag))
        self.assertEqual(self.writes, [])

    def test_newer_draft_prerelease_and_other_major_do_not_block(self):
        self.responses[f"{BASE}/releases?per_page=100"] += [
            release("v5.5.0", draft=True),
            release("v5.6.0", prerelease=True),
            release("v5.7.0-rc.1", prerelease=True),
            release("v6.0.0"),
        ]
        self.run_update()
        self.assertEqual(self.writes, self.pushed())

    def test_retargeted_immutable_release_fails_closed(self):
        self.responses[f"{BASE}/git/tags/{ANNOTATION}"] = {"object": {"type": "commit", "sha": PREVIOUS}}
        with self.assertRaisesRegex(ValueError, "publishing run"):
            self.run_update()
        self.assertEqual(self.writes, [])

    def test_prefix_match_is_not_existing_floating_tag(self):
        self.responses[f"{BASE}/git/matching-refs/tags/v5"] = [
            {"ref": "refs/tags/v5.3.0", "object": {"type": "commit", "sha": PREVIOUS}},
        ]
        self.run_update()
        self.assertEqual(self.writes, self.pushed(lease=""))

    def test_api_failure_never_mutates(self):
        def failing_api(*_args, **_kwargs):
            raise RuntimeError("unavailable")

        with self.assertRaisesRegex(RuntimeError, "unavailable"):
            MODULE.advance("owner/repo", "v5.4.0", SHA, api=failing_api, git=self.git)
        self.assertEqual(self.writes, [])

    def test_gh_api_flattens_every_page(self):
        with patch.object(MODULE.subprocess, "run") as run:
            run.return_value.stdout = '[[{"tag_name":"v5.10.0"}],[{"tag_name":"v5.4.0"}]]'
            self.assertEqual(
                MODULE.gh_api(f"{BASE}/releases?per_page=100", paginate=True),
                [{"tag_name": "v5.10.0"}, {"tag_name": "v5.4.0"}],
            )
            self.assertIn("--paginate", run.call_args.args[0])
            self.assertIn("--slurp", run.call_args.args[0])

    def test_git_env_sends_the_token_only_through_the_environment(self):
        env = MODULE.git_env({"GH_TOKEN": "secret-value", "GITHUB_SERVER_URL": "https://ghe.example/"})
        self.assertEqual(env["GIT_CONFIG_COUNT"], "2")
        self.assertEqual(env["GIT_CONFIG_KEY_0"], "http.https://ghe.example/.extraheader")
        self.assertEqual(env["GIT_CONFIG_VALUE_0"], "")
        self.assertEqual(env["GIT_CONFIG_KEY_1"], env["GIT_CONFIG_KEY_0"])
        self.assertNotIn("secret-value", env["GIT_CONFIG_VALUE_1"])
        self.assertTrue(env["GIT_CONFIG_VALUE_1"].startswith("AUTHORIZATION: basic "))
        self.assertEqual(env["GIT_COMMITTER_NAME"], "github-actions[bot]")

    def test_git_env_without_token_adds_no_header(self):
        self.assertNotIn("GIT_CONFIG_COUNT", MODULE.git_env({}))

    def test_run_git_keeps_the_token_off_the_command_line(self):
        with (
            patch.dict(MODULE.os.environ, {"GH_TOKEN": "secret-value"}),
            patch.object(MODULE.subprocess, "run") as run,
        ):
            MODULE.run_git(["push", "origin", "refs/tags/v5:refs/tags/v5"])
            self.assertEqual(run.call_args.args[0], ["git", "push", "origin", "refs/tags/v5:refs/tags/v5"])
            self.assertIn("GIT_CONFIG_VALUE_1", run.call_args.kwargs["env"])


if __name__ == "__main__":
    unittest.main()
