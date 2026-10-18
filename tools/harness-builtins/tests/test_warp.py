import unittest

from tests import helpers


class WarpTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("warp")

    def test_parse_commands_reads_rows_that_span_lines(self):
        # Arrange
        text = helpers.read("warp", "slash-commands.md")
        # Act
        got = self.ex.parse_commands(text, "Static slash commands", "Slash Command")
        # Assert
        self.assertEqual(got, ["add-mcp", "agent", "fork", "fork-and-compact", "new", "plan", "skills"])

    def test_parse_commands_reads_the_cli_table(self):
        # Arrange
        text = helpers.read("warp", "cli-reference.md")
        # Act
        got = self.ex.parse_commands(text, "Slash commands", "Command")
        # Assert
        self.assertEqual(got, ["agent", "api-keys", "clear", "exit", "fork", "new", "plan", "theme", "voice"])

    def test_parse_commands_fails_when_the_heading_is_gone(self):
        # Arrange / Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_commands("# Page\n\nNo tables.\n", "Slash commands", "Command")

    def test_extract_leaves_the_cli_group_the_names_the_app_does_not_list(self):
        # Arrange
        ctx = helpers.context(
            files={
                self.ex.APP: helpers.read("warp", "slash-commands.md"),
                self.ex.CLI: helpers.read("warp", "cli-reference.md"),
            }
        )
        # Act
        got = self.ex.extract(ctx)
        # Assert
        self.assertEqual(got["cli"]["names"], ["api-keys", "clear", "exit", "theme", "voice"])
        self.assertIn("agent", got["app"]["names"])
