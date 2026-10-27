import unittest

from tests import helpers


class ClaudeTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("claude")

    def test_parse_commands_splits_commands_from_bundled_skills_and_workflows(self):
        # Arrange
        text = helpers.read("claude", "commands.md")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertIn("add-dir", got["command"]["names"])
        self.assertIn("usage-credits", got["command"]["names"])
        self.assertIn("ultrareview", got["command"]["names"])
        self.assertEqual(got["skill"]["names"], ["batch", "code-review", "deep-research", "doctor", "loop"])

    def test_parse_commands_reads_every_alias_phrasing_and_skips_alias_rows_as_names(self):
        # Arrange
        text = helpers.read("claude", "commands.md")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        aliases = got["command"]["aliases"]
        self.assertEqual(aliases["background"], ["bg"])
        self.assertEqual(aliases["clear"], ["new", "reset"])
        self.assertEqual(aliases["rewind"], ["checkpoint", "undo"])
        self.assertEqual(aliases["mobile"], ["android", "ios"])
        self.assertEqual(aliases["usage"], ["cost", "stats"])
        self.assertEqual(aliases["bug"], ["share"])
        self.assertNotIn("cost", got["command"]["names"])
        self.assertNotIn("stats", got["command"]["names"])
        self.assertEqual(got["skill"]["aliases"]["code-review"], ["review"])
        self.assertNotIn("review", got["skill"]["names"])
        self.assertEqual(got["skill"]["aliases"]["doctor"], ["checkup"])

    def test_parse_commands_does_not_take_a_historical_alias_as_current(self):
        # Arrange
        text = (
            "| `/bug [r]` | Report. Alias: `/share`. Before v2.1.1, `/bug` and `/share` were aliases of `/feedback` |\n"
        )
        # Act
        got = self.ex.parse_commands(text + "| `/feedback` | Send |\n| `/s` | **[Skill](/x).** Run |\n")
        # Assert
        self.assertEqual(got["command"]["aliases"], {"bug": ["share"]})

    def test_a_subcommand_row_does_not_change_the_kind_of_its_command(self):
        # Arrange
        text = (
            "| `/design [brief]` | **[Skill](/x).** Draft mockups |\n"
            "| `/design consent`, `/design revoke` | Grant access, or withdraw it |\n"
            "| `/a` | Plain |\n"
        )
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertEqual(got["skill"]["names"], ["design"])
        self.assertEqual(got["command"]["names"], ["a"])

    def test_an_alias_of_a_row_that_does_not_exist_is_an_error(self):
        # Arrange
        text = "| `/a` | Plain |\n| `/b` | **[Skill](/x).** s |\n| `/c` | Alias for `/missing` |\n"
        # Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_commands(text)

    def test_parse_subagents_reads_tabs_and_table_and_the_documented_override(self):
        # Arrange
        text = helpers.read("claude", "sub-agents.md")
        # Act
        got = self.ex.parse_subagents(text)
        # Assert
        self.assertEqual(got["agent_override"], ["Explore"])
        self.assertEqual(got["agent"], ["Plan", "General-purpose", "claude", "statusline-setup", "claude-code-guide"])

    def test_parse_subagents_reads_the_earlier_tab_layout(self):
        # Arrange
        text = (
            '## Built-in subagents\n\n<Tabs>\n  <Tab title="Explore">\n'
            "    A [project subagent](#x) named `Explore` overrides the built-in.\n  </Tab>\n"
            '  <Tab title="Plan">\n    Research.\n  </Tab>\n  <Tab title="Other">\n'
            "    | Agent | Model | When Claude uses it |\n    | :- | :- | :- |\n"
            "    | statusline-setup | Sonnet | When you run `/statusline` |\n  </Tab>\n</Tabs>\n"
        )
        # Act
        got = self.ex.parse_subagents(text)
        # Assert
        self.assertEqual(got["agent_override"], ["Explore"])
        self.assertEqual(got["agent"], ["Plan", "statusline-setup"])

    def test_extract_fetches_the_two_pages(self):
        # Arrange
        ctx = helpers.context(
            files={
                self.ex.COMMANDS: helpers.read("claude", "commands.md"),
                self.ex.SUBAGENTS: helpers.read("claude", "sub-agents.md"),
            }
        )
        # Act
        got = self.ex.extract(ctx)
        # Assert
        self.assertEqual(sorted(got), ["agent", "agent_override", "command", "skill"])

    def test_a_changed_page_is_an_error_not_an_empty_result(self):
        # Arrange / Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_commands("# Commands\n\nno table\n")
        with self.assertRaises(ValueError):
            self.ex.parse_commands("| `/a` | **[Skill](/x).** only skills |\n")
        with self.assertRaises(ValueError):
            self.ex.parse_subagents("## Something\n")
        with self.assertRaises(ValueError):
            self.ex.parse_subagents("## Built-in subagents\n\nprose\n")


if __name__ == "__main__":
    unittest.main()
