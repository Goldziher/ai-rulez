import unittest

from tests import helpers


class QoderTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("qoder")

    def test_parse_commands_skips_the_built_in_skills_table(self):
        # Arrange
        text = helpers.read("qoder", "slash-reference.md")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertEqual(
            got["names"], ["new", "clear", "quit", "plan", "tasks", "workflows", "settings", "theme", "login", "logout"]
        )

    def test_parse_commands_reads_aliases_from_the_description(self):
        # Arrange
        text = helpers.read("qoder", "slash-reference.md")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertEqual(
            got["aliases"],
            {
                "tasks": ["bg", "background"],
                "workflows": ["workflow-tasks"],
                "settings": ["config"],
                "login": ["signin"],
                "logout": ["signout"],
            },
        )

    def test_parse_commands_fails_without_a_command_table(self):
        # Arrange / Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_commands("# Slash Commands\n\nNothing here.\n")

    def test_parse_builtins_reads_agents_and_skills(self):
        # Arrange
        text = helpers.read("qoder", "builtins-reference.md")
        # Act
        agents = self.ex.parse_builtins(text, "Agent")
        skills = self.ex.parse_builtins(text, "Skill")
        # Assert
        self.assertEqual(agents, ["general-purpose", "Explore", "qoder-guide"])
        self.assertEqual(skills, ["loop", "run", "workflow-authoring"])

    def test_parse_builtins_fails_when_the_table_is_renamed(self):
        # Arrange
        text = helpers.read("qoder", "builtins-reference.md").replace("| Agent ", "| Subagent ")
        # Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_builtins(text, "Agent")

    def test_extract_returns_all_three_keys(self):
        # Arrange
        ctx = helpers.context(
            files={
                self.ex.SLASH: helpers.read("qoder", "slash-reference.md"),
                self.ex.BUILTINS: helpers.read("qoder", "builtins-reference.md"),
            }
        )
        # Act
        got = self.ex.extract(ctx)
        # Assert
        self.assertEqual(set(got), {"command", "skill", "agent"})
