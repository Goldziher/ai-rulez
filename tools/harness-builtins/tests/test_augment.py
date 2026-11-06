import unittest

from tests import helpers


class AugmentTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("augment")

    def test_parse_commands_reads_the_table_and_ignores_the_shortcuts_table(self):
        # Arrange
        text = helpers.read("augment", "interactive.md")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertIn("about", got["names"])
        self.assertIn("restart-mcp", got["names"])
        self.assertNotIn("Ctrl + P", got["names"])
        self.assertEqual(len(got["names"]), 35)

    def test_parse_commands_takes_every_command_of_a_row(self):
        # Arrange
        text = helpers.read("augment", "interactive.md")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        for name in ("exit", "quit", "login", "logout"):
            self.assertIn(name, got["names"])

    def test_parse_commands_strips_arguments(self):
        # Arrange
        text = helpers.read("augment", "interactive.md")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertIn("name", got["names"])
        self.assertIn("import", got["names"])
        self.assertNotIn("name <name>", got["names"])

    def test_parse_commands_rejects_a_changed_layout(self):
        # Arrange
        text = "### Slash Commands\n\nNo tables any more.\n\n#### Custom and skill slash commands\n"
        # Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_commands(text)

    def test_extract_reads_the_page(self):
        # Arrange
        ctx = helpers.context(files={self.ex.URL: helpers.read("augment", "interactive.md")})
        # Act
        got = self.ex.extract(ctx)
        # Assert
        self.assertEqual(set(got), {"command"})


if __name__ == "__main__":
    unittest.main()
