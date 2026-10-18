import unittest

from tests import helpers


class CommandCodeTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("commandcode")

    def test_parse_commands_reads_names_through_arguments_and_parenthesised_aliases(self):
        # Arrange
        text = helpers.read("commandcode", "slash-commands.html")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        for name in ("clear", "resume", "export", "exit", "mode", "mode:plan", "plan", "feedback", "help"):
            self.assertIn(name, got["names"])
        self.assertNotIn("new", got["names"])
        self.assertNotIn("issue", got["names"])

    def test_parse_commands_reads_aliases(self):
        # Arrange
        text = helpers.read("commandcode", "slash-commands.html")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertEqual(got["aliases"]["clear"], ["new"])
        self.assertEqual(got["aliases"]["exit"], ["quit"])
        self.assertEqual(got["aliases"]["feedback"], ["issue"])
        self.assertEqual(got["aliases"]["rename"], ["name"])  # "Alias of /rename"; /name stays a name
        self.assertIn("name", got["names"])

    def test_parse_commands_repeating_the_command_in_one_cell_is_not_an_alias(self):
        # Arrange
        text = helpers.read("commandcode", "slash-commands.html")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertNotIn("export", got["aliases"])

    def test_parse_commands_skips_tables_with_other_headers(self):
        # Arrange
        text = helpers.read("commandcode", "slash-commands.html")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertNotIn("mods", got["names"])  # the "Command | Effect" table of mod commands

    def test_parse_commands_rejects_a_page_without_the_tables(self):
        # Arrange
        text = "<html><body><p>Moved</p></body></html>"
        # Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_commands(text)

    def test_extract_reads_the_page(self):
        # Arrange
        ctx = helpers.context(files={self.ex.URL: helpers.read("commandcode", "slash-commands.html")})
        # Act
        got = self.ex.extract(ctx)
        # Assert
        self.assertEqual(set(got), {"command"})


if __name__ == "__main__":
    unittest.main()
