import unittest

from tests import helpers


class KimiTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("kimi")

    def test_parse_commands_reads_names_aliases_and_ignores_the_goal_actions_table(self):
        # Arrange
        text = helpers.read("kimi", "slash-commands.md")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertEqual(len(got["names"]), 40)
        for name in ("login", "title", "compact", "plan", "swarm", "goal", "exit", "export-md"):
            self.assertIn(name, got["names"])
        self.assertEqual(len(set(got["names"])), len(got["names"]))
        self.assertEqual(got["aliases"]["help"], ["?", "h"])
        self.assertEqual(got["aliases"]["exit"], ["q", "quit"])
        self.assertEqual(got["aliases"]["new"], ["clear"])
        self.assertNotIn("mcp-config", got["names"])

    def test_parse_skill_commands_includes_the_dotted_sub_skills(self):
        # Arrange
        text = helpers.read("kimi", "slash-commands.md")
        # Act
        got = self.ex.parse_skill_commands(text)
        # Assert
        self.assertEqual(
            got,
            [
                "mcp-config",
                "custom-theme",
                "update-config",
                "check-kimi-code-docs",
                "import-from-cc-codex",
                "sub-skill",
                "sub-skill.review",
                "sub-skill.consolidate",
            ],
        )

    def test_parse_subagents_reads_the_bullets_of_the_section(self):
        # Arrange
        text = helpers.read("kimi", "agents.md")
        # Act
        got = self.ex.parse_subagents(text)
        # Assert
        self.assertEqual(got, ["coder", "explore", "plan"])

    def test_extract_fetches_the_two_pages(self):
        # Arrange
        ctx = helpers.context(
            files={
                self.ex.COMMANDS: helpers.read("kimi", "slash-commands.md"),
                self.ex.AGENTS: helpers.read("kimi", "agents.md"),
            }
        )
        # Act
        got = self.ex.extract(ctx)
        # Assert
        self.assertEqual(sorted(got), ["agent", "command", "skill"])

    def test_a_changed_page_is_an_error_not_an_empty_result(self):
        # Arrange / Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_commands("# Slash Commands\n\nno tables\n")
        with self.assertRaises(ValueError):
            self.ex.parse_skill_commands("## Something else\n")
        with self.assertRaises(ValueError):
            self.ex.parse_subagents("## Built-in Sub-Agents\n\nprose only\n\n## Next\n")
        with self.assertRaises(ValueError):
            self.ex.parse_commands("| Command | Alias |\n| --- | --- |\n| login | — |\n")


if __name__ == "__main__":
    unittest.main()
