import unittest

from tests import helpers


class JunieTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("junie")

    def test_parse_commands_reads_the_built_in_table_only(self):
        # Arrange
        text = helpers.read("junie", "slash-commands.html")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertEqual(
            got["names"], ["account", "branch", "history", "quit", "title", "stats", "compress", "logs", "voice"]
        )

    def test_parse_commands_reads_alias_sentences(self):
        # Arrange
        text = helpers.read("junie", "slash-commands.html")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertEqual(got["aliases"]["logs"], ["export-logs"])
        self.assertEqual(got["aliases"]["voice"], ["mic", "record"])

    def test_parse_commands_fails_when_the_table_is_gone(self):
        # Arrange / Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_commands("<html><body><h2>Slash commands</h2><p>moved</p></body></html>")

    def test_extract_returns_the_command_key(self):
        # Arrange
        ctx = helpers.context(files={self.ex.PAGE: helpers.read("junie", "slash-commands.html")})
        # Act
        got = self.ex.extract(ctx)
        # Assert
        self.assertEqual(list(got), ["command"])
