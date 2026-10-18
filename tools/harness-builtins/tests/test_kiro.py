import unittest

from tests import helpers


class KiroTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("kiro")

    def test_parse_cli_reads_command_headings_and_skips_subcommands_and_other_sections(self):
        # Arrange
        text = helpers.read("kiro", "reference-slash-commands.md")
        # Act
        got = self.ex.parse_cli(text)
        # Assert
        self.assertEqual(got["names"], ["help", "quit", "agent", "mcp", "changelog"])

    def test_parse_cli_reads_the_aliases_line(self):
        # Arrange
        text = helpers.read("kiro", "reference-slash-commands.md")
        # Act
        got = self.ex.parse_cli(text)
        # Assert
        self.assertEqual(got["aliases"], {"quit": ["exit", "q"]})

    def test_parse_cli_fails_without_command_headings(self):
        # Arrange / Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_cli("## Available commands\n\nSee the app.\n")

    def test_parse_ide_reads_the_predefined_steering_table(self):
        # Arrange
        text = helpers.read("kiro", "ide-slash-commands.md")
        # Act
        got = self.ex.parse_ide(text)
        # Assert
        self.assertEqual(got, ["architecture-selection", "quick-spec", "bug-fix"])

    def test_parse_ide_fails_when_the_table_is_gone(self):
        # Arrange / Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_ide("### Steering files\n\nNo table.\n")

    def test_extract_returns_both_keys(self):
        # Arrange
        ctx = helpers.context(
            files={
                self.ex.CLI: helpers.read("kiro", "reference-slash-commands.md"),
                self.ex.IDE: helpers.read("kiro", "ide-slash-commands.md"),
            }
        )
        # Act
        got = self.ex.extract(ctx)
        # Assert
        self.assertEqual(set(got), {"cli", "ide"})
