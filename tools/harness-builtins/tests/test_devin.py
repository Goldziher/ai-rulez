import unittest

from tests import helpers


class DevinTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("devin")

    def test_parse_commands_reads_every_table_of_the_section(self):
        # Arrange
        text = helpers.read("devin", "commands.md")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        for name in ("mode", "theme", "resume", "workspace", "loop", "help", "cloud", "archive"):
            self.assertIn(name, got["names"])
        self.assertEqual(len(got["names"]), 57)

    def test_parse_commands_reads_aliases_from_descriptions_and_the_note(self):
        # Arrange
        text = helpers.read("devin", "commands.md")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertEqual(got["aliases"]["clear"], ["new"])
        self.assertEqual(got["aliases"]["exit"], ["quit"])
        self.assertEqual(got["aliases"]["bypass"], ["yolo", "dangerous"])
        self.assertEqual(got["aliases"]["undo-add-dir"], ["remove-dir"])
        self.assertIn("remove-dir", got["names"])  # still a row of its own
        self.assertNotIn("login-status", got["aliases"])  # "advertised ... as /status" is not an alias

    def test_parse_commands_rejects_a_page_without_the_section(self):
        # Arrange
        text = "## Usage\n\nnothing\n"
        # Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_commands(text)

    def test_parse_profiles_reads_the_profile_names(self):
        # Arrange
        text = helpers.read("devin", "subagents.md")
        # Act
        got = self.ex.parse_profiles(text)
        # Assert
        self.assertEqual(got["names"], ["subagent_explore", "subagent_general"])

    def test_parse_profiles_rejects_a_missing_table(self):
        # Arrange
        text = "## Subagent Profiles\n\nProse only.\n"
        # Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_profiles(text)

    def test_extract_reads_both_pages(self):
        # Arrange
        ctx = helpers.context(
            files={
                self.ex.COMMANDS_URL: helpers.read("devin", "commands.md"),
                self.ex.SUBAGENTS_URL: helpers.read("devin", "subagents.md"),
            }
        )
        # Act
        got = self.ex.extract(ctx)
        # Assert
        self.assertEqual(set(got), {"command", "agent"})


if __name__ == "__main__":
    unittest.main()
