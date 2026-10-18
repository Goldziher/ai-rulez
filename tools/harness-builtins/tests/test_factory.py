import unittest

from tests import helpers


class FactoryTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("factory")

    def test_parse_commands_reads_the_slash_command_table(self):
        # Arrange
        text = helpers.read("factory", "cli-reference.md")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertEqual(got["names"], ["cd", "cwd", "new", "plugins", "quit", "stats"])

    def test_parse_commands_takes_an_alias_clause_but_not_alias_for(self):
        # Arrange
        text = helpers.read("factory", "cli-reference.md")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertEqual(got["aliases"], {"quit": ["exit"]})

    def test_parse_commands_ignores_the_shell_command_table(self):
        # Arrange
        text = helpers.read("factory", "cli-reference.md")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertNotIn("droid", got["names"])

    def test_parse_commands_fails_when_the_section_is_gone(self):
        # Arrange / Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_commands("## Droid CLI commands\n\n| Command | Description |\n| - | - |\n| `droid` | x |\n")

    def test_extract_returns_the_command_key(self):
        # Arrange
        ctx = helpers.context(files={self.ex.REFERENCE: helpers.read("factory", "cli-reference.md")})
        # Act
        got = self.ex.extract(ctx)
        # Assert
        self.assertEqual(list(got), ["command"])
