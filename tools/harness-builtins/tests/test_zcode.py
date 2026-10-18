import unittest

from tests import helpers


class ZcodeTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("zcode")

    def test_parse_commands_reads_the_command_purpose_table(self):
        # Arrange
        text = helpers.read("zcode", "commands.html")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertEqual(got, ["goal", "compact"])

    def test_parse_commands_ignores_the_custom_command_field_table(self):
        # Arrange
        text = helpers.read("zcode", "commands.html")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertNotIn("command-name", got)

    def test_parse_commands_fails_when_the_table_is_renamed(self):
        # Arrange
        text = helpers.read("zcode", "commands.html").replace("Purpose", "Use")
        # Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_commands(text)

    def test_extract_returns_the_command_key(self):
        # Arrange
        ctx = helpers.context(files={self.ex.PAGE: helpers.read("zcode", "commands.html")})
        # Act
        got = self.ex.extract(ctx)
        # Assert
        self.assertEqual(got, {"command": {"names": ["goal", "compact"]}})
