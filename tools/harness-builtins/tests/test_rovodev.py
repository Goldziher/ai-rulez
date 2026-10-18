import unittest

from tests import helpers


class RovodevTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("rovodev")

    def test_parse_commands_takes_the_first_word_of_each_form_once(self):
        # Arrange
        text = helpers.read("rovodev", "commands.html")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertEqual(got["names"], ["new", "sessions", "clear", "prompts", "memory", "copy", "plan", "exit"])

    def test_parse_commands_ignores_escaped_json_copies_and_cli_flags(self):
        # Arrange
        text = helpers.read("rovodev", "commands.html")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertNotIn("jira", got["names"])
        self.assertNotIn("acli", got["names"])

    def test_parse_commands_reads_the_also_clause_as_aliases(self):
        # Arrange
        text = helpers.read("rovodev", "commands.html")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertEqual(got["aliases"], {"exit": ["quit", "q"]})

    def test_parse_commands_fails_without_the_interactive_mode_heading(self):
        # Arrange
        text = helpers.read("rovodev", "commands.html").replace("Interactive mode</h2>", "Modes</h2>")
        # Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_commands(text)

    def test_extract_returns_the_command_key(self):
        # Arrange
        ctx = helpers.context(files={self.ex.PAGE: helpers.read("rovodev", "commands.html")})
        # Act
        got = self.ex.extract(ctx)
        # Assert
        self.assertEqual(list(got), ["command"])
