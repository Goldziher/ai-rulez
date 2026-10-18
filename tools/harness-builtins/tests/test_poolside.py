import unittest

from tests import helpers


class PoolsideTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("poolside")

    def test_parse_commands_reads_every_slash_command_table_once(self):
        # Arrange
        text = helpers.read("poolside", "cli-reference.md")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertEqual(
            got["names"],
            ["model", "agent-mode", "thought-level", "new", "quit", "debug:dump", "plan", "skills", "clear"],
        )

    def test_parse_commands_reads_or_and_as_an_alias_forms(self):
        # Arrange
        text = helpers.read("poolside", "cli-reference.md")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertEqual(
            got["aliases"],
            {"agent-mode": ["collaboration-mode"], "thought-level": ["effort"], "new": ["clear"], "quit": ["exit"]},
        )

    def test_parse_commands_ignores_the_flag_tables(self):
        # Arrange
        text = helpers.read("poolside", "cli-reference.md")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertNotIn("directory", got["names"])

    def test_parse_commands_fails_when_the_tables_are_gone(self):
        # Arrange / Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_commands("## Slash commands\n\nSee the app.\n")

    def test_extract_returns_the_command_key(self):
        # Arrange
        ctx = helpers.context(files={self.ex.REFERENCE: helpers.read("poolside", "cli-reference.md")})
        # Act
        got = self.ex.extract(ctx)
        # Assert
        self.assertEqual(list(got), ["command"])
