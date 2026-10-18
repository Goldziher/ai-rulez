import json
import unittest

from tests import helpers


class GitlabDuoTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("gitlab_duo")

    def test_parse_handler_reads_name_and_aliases(self):
        # Arrange
        text = helpers.read("gitlab-duo", "new_session_command_handler.ts")
        # Act
        got = self.ex.parse_handler(text)
        # Assert
        self.assertEqual(got, ("new", ["clear"]))

    def test_parse_handler_ignores_the_doc_comment_and_a_commented_out_command(self):
        # Arrange
        text = helpers.read("gitlab-duo", "disabled_command_handler.ts")
        # Act
        got = self.ex.parse_handler(text)
        # Assert
        self.assertIsNone(got)

    def test_handler_files_keeps_sources_and_drops_tests_and_directories(self):
        # Arrange
        listing = helpers.read("gitlab-duo", "tree.json")
        # Act
        got = self.ex.handler_files(listing)
        # Assert
        self.assertEqual(
            [p.rsplit("/", 1)[1] for p in got],
            ["create_session_helpers.ts", "doctor_command_handler.ts", "exit_command_handler.ts"],
        )

    def test_extract_reads_every_handler_at_the_pinned_commit(self):
        # Arrange
        ref = "abc123"
        base = f"{self.ex.RAW}/{ref}/{self.ex.DIR}"
        files = {
            f"{self.ex.API}/repository/tree?path={self.ex.DIR}&ref={ref}&per_page=100": helpers.read(
                "gitlab-duo", "tree.json"
            ),
            f"{base}/create_session_helpers.ts": "export const x = 1;\n",
            f"{base}/doctor_command_handler.ts": helpers.read("gitlab-duo", "doctor_command_handler.ts"),
            f"{base}/exit_command_handler.ts": helpers.read("gitlab-duo", "exit_command_handler.ts"),
        }
        ctx = helpers.context(ref=ref, files=files)
        # Act
        got = self.ex.extract(ctx)
        # Assert
        self.assertEqual(got, {"command": {"names": ["doctor", "exit"], "aliases": {"exit": ["quit"]}}})

    def test_extract_requires_a_ref(self):
        # Arrange / Act / Assert
        with self.assertRaises(ValueError):
            self.ex.extract(helpers.context())

    def test_extract_fails_when_no_handler_declares_a_command(self):
        # Arrange
        ref = "abc123"
        files = {
            f"{self.ex.API}/repository/tree?path={self.ex.DIR}&ref={ref}&per_page=100": json.dumps([]),
        }
        # Act / Assert
        with self.assertRaises(ValueError):
            self.ex.extract(helpers.context(ref=ref, files=files))
