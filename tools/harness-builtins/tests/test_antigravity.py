import unittest

from tests import helpers


class AntigravityTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("antigravity")

    def test_parse_commands_reads_names_through_links_bold_and_arguments(self):
        # Arrange
        text = helpers.read("antigravity", "reference.md")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertEqual(
            got["names"],
            ["add-dir", "agents", "btw", "clear", "copy", "exit", "fork", "resume", "teamwork-preview", "voice"],
        )

    def test_parse_commands_reads_the_alias_column(self):
        # Arrange
        text = helpers.read("antigravity", "reference.md")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertEqual(got["aliases"]["resume"], ["switch", "conversation"])
        self.assertEqual(got["aliases"]["exit"], ["quit"])
        self.assertNotIn("agents", got["aliases"])  # "—" means none

    def test_parse_commands_ignores_the_keybinding_table(self):
        # Arrange
        text = helpers.read("antigravity", "reference.md")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertEqual(got["names"].count("exit"), 1)

    def test_parse_commands_rejects_a_page_without_the_table(self):
        # Arrange
        text = "# CLI commands\n\n## Core slash commands\n\nMoved.\n"
        # Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_commands(text)

    def test_extract_reads_the_page(self):
        # Arrange
        ctx = helpers.context(files={self.ex.URL: helpers.read("antigravity", "reference.md")})
        # Act
        got = self.ex.extract(ctx)
        # Assert
        self.assertIn("voice", got["command"]["names"])


if __name__ == "__main__":
    unittest.main()
