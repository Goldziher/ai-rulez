import unittest

from tests import helpers


class HermesTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("hermes")

    def test_parse_registry_reads_names_and_aliases(self):
        # Arrange
        text = helpers.read("hermes", "commands.py")
        # Act
        got = self.ex.parse_registry(text)
        # Assert
        self.assertEqual(len(got["names"]), 103)
        self.assertEqual(got["names"][:3], ["start", "new", "topic"])
        self.assertEqual(got["aliases"]["new"], ["reset"])
        self.assertEqual(got["aliases"]["quit"], ["exit"])
        self.assertEqual(got["aliases"]["reload-mcp"], ["reload_mcp"])
        self.assertNotIn("topic", got["aliases"])

    def test_parse_registry_keeps_cli_only_and_gateway_only_commands(self):
        # Arrange
        text = helpers.read("hermes", "commands.py")
        # Act
        got = self.ex.parse_registry(text)
        # Assert
        self.assertIn("clear", got["names"])  # cli_only
        self.assertIn("restart", got["names"])  # gateway_only

    def test_slugify_matches_the_command_a_skill_name_registers(self):
        # Arrange / Act / Assert
        self.assertEqual(self.ex.slugify("Foo Bar"), "foo-bar")
        self.assertEqual(self.ex.slugify("git_helper"), "git-helper")
        self.assertEqual(self.ex.slugify("c++/tools"), "ctools")

    def test_skill_paths_skip_hidden_scan_directories(self):
        # Arrange
        paths = ["skills/a/x/SKILL.md", "skills/.hub/y/SKILL.md", "skills/a/x/references/notes.md", "skills/AGENTS.md"]
        # Act
        got = self.ex.skill_paths(paths)
        # Assert
        self.assertEqual(got, ["skills/a/x/SKILL.md"])

    def test_extract_reads_the_skill_name_from_the_front_matter(self):
        # Arrange
        ctx = helpers.context(
            "NousResearch/hermes-agent",
            "c1",
            files={
                self.ex.COMMANDS: helpers.read("hermes", "commands.py"),
                "skills/apple/apple-notes/SKILL.md": helpers.read("hermes", "apple-notes_SKILL.md"),
                "skills/web/dir-name/SKILL.md": "no front matter\n",
            },
            tree=["skills/apple/apple-notes/SKILL.md", "skills/web/dir-name/SKILL.md", "skills/index-cache/x.json"],
        )
        # Act
        got = self.ex.extract(ctx)
        # Assert
        self.assertEqual(got["skill"]["names"], ["apple-notes", "dir-name"])
        self.assertIn("goal", got["command"]["names"])

    def test_a_changed_format_is_an_error_not_an_empty_result(self):
        # Arrange / Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_registry("COMMAND_REGISTRY = []\n")
        with self.assertRaises(ValueError):
            self.ex.parse_registry("def (:\n")
        with self.assertRaises(ValueError):
            self.ex.parse_registry("COMMAND_REGISTRY: list = [make('x')]\n")
        with self.assertRaises(ValueError):
            self.ex.skill_paths(["README.md"])


if __name__ == "__main__":
    unittest.main()
