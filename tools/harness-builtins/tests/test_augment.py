import unittest

from tests import helpers


class AugmentTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("augment")

    def test_parse_commands_reads_both_tables_and_ignores_the_shortcuts_table(self):
        # Arrange
        text = helpers.read("augment", "interactive.md")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertIn("about", got["names"])  # common table
        self.assertIn("restart-mcp", got["names"])  # additional table
        self.assertNotIn("Ctrl + P", got["names"])
        self.assertEqual(len(got["names"]), 44)

    def test_parse_commands_strips_arguments(self):
        # Arrange
        text = helpers.read("augment", "interactive.md")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertIn("btw", got["names"])
        self.assertIn("image", got["names"])
        self.assertNotIn("btw <question>", got["names"])

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
