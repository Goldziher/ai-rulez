import unittest

from tests import helpers


class GrokTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("grok")

    def test_parse_commands_reads_both_tables_with_aliases(self):
        # Arrange
        text = helpers.read("grok", "modes-and-commands.md")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertEqual(
            got["names"],
            [
                "quit",
                "help",
                "new",
                "rename",
                "model",
                "settings",
                "login",
                "logout",
                "skills",
                "mcps",
                "goal",
                "flush",
                "memory",
                "dream",
            ],
        )
        self.assertEqual(got["aliases"]["quit"], ["exit"])
        self.assertEqual(got["aliases"]["rename"], ["title"])
        self.assertEqual(got["aliases"]["memory"], ["mem"])

    def test_parse_commands_ignores_the_goal_subcommand_table(self):
        # Arrange
        text = helpers.read("grok", "modes-and-commands.md")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertEqual(got["names"].count("goal"), 1)

    def test_parse_commands_fails_when_a_table_is_missing(self):
        # Arrange
        text = "## Core TUI commands\n\n| Command | What it does |\n| - | - |\n| `/help` | Help |\n"
        # Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_commands(text)

    def test_parse_agents_reads_the_built_in_types(self):
        # Arrange
        text = helpers.read("grok", "subagents.md")
        # Act
        got = self.ex.parse_agents(text)
        # Assert
        self.assertEqual(got, ["general-purpose", "explore", "plan"])

    def test_parse_agents_fails_when_the_table_is_gone(self):
        # Arrange / Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_agents("# Subagents\n\nNo table.\n")

    def test_extract_returns_both_keys(self):
        # Arrange
        ctx = helpers.context(
            files={
                self.ex.COMMANDS: helpers.read("grok", "modes-and-commands.md"),
                self.ex.AGENTS: helpers.read("grok", "subagents.md"),
            }
        )
        # Act
        got = self.ex.extract(ctx)
        # Assert
        self.assertEqual(set(got), {"command", "agent"})
