import unittest

from tests import helpers


class CopilotCliTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("copilot_cli")

    def test_parse_commands_takes_the_first_command_and_the_rest_as_aliases(self):
        # Arrange
        text = helpers.read("copilot-cli", "cli-command-reference.md")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertEqual(got["aliases"]["ask"], ["btw"])
        self.assertEqual(got["aliases"]["allow-all"], ["yolo"])  # the escaped pipes in the cell do not split it
        self.assertEqual(got["aliases"]["clear"], ["new", "reset"])
        self.assertEqual(got["aliases"]["model"], ["models"])
        self.assertEqual(got["aliases"]["cwd"], ["cd"])

    def test_parse_commands_counts_repeats_once_and_ignores_subcommand_rows(self):
        # Arrange
        text = helpers.read("copilot-cli", "cli-command-reference.md")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertEqual(got["names"].count("after"), 1)  # "`/after [DELAY PROMPT]`, `/after`"
        self.assertEqual(got["names"].count("worktree"), 1)  # "/worktree new" is the same command
        self.assertIn("reset-allowed-tools", got["names"])
        self.assertNotIn("after", got["aliases"])

    def test_parse_names_reads_the_built_in_agents(self):
        # Arrange
        text = helpers.read("copilot-cli", "cli-command-reference.md")
        # Act
        got = self.ex.parse_names(text, "Built-in agents", "Agent")
        # Assert
        self.assertEqual(
            got["names"],
            ["code-review", "explore", "general-purpose", "research", "rubber-duck", "security-review", "task"],
        )

    def test_parse_names_reads_the_built_in_skill_ids(self):
        # Arrange
        text = helpers.read("copilot-cli", "built-in-skills.md")
        # Act
        got = self.ex.parse_names(text, "", "Skill ID")
        # Assert
        self.assertEqual(
            got["names"], ["af", "agent-merge", "create-canvas", "orchestrate", "pr-stack", "customize-cloud-agent"]
        )

    def test_parse_commands_rejects_a_missing_section(self):
        # Arrange
        text = "## Command-line options\n\nnothing\n"
        # Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_commands(text)

    def test_parse_names_rejects_a_row_without_a_name(self):
        # Arrange
        text = "### Built-in agents\n\n| Agent | Model |\n|---|---|\n| plain | x |\n"
        # Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_names(text, "Built-in agents", "Agent")

    def test_extract_reads_the_pinned_markdown_sources(self):
        # Arrange
        ctx = helpers.context(
            "github/docs",
            "abc123",
            files={
                self.ex.REFERENCE: helpers.read("copilot-cli", "cli-command-reference.md"),
                self.ex.SKILLS: helpers.read("copilot-cli", "built-in-skills.md"),
            },
        )
        # Act
        got = self.ex.extract(ctx)
        # Assert
        self.assertEqual(set(got), {"command", "agent", "skill"})
        self.assertIn("task", got["agent"]["names"])


if __name__ == "__main__":
    unittest.main()
