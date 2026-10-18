import unittest

from tests import helpers

SK = "internal/skills/builtin/"


class CrushTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("crush")

    def test_parse_commands_reads_literal_ids_and_aliases_but_not_dynamic_ones(self):
        # Arrange
        text = helpers.read("crush", "commands.go")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertEqual(
            got["names"],
            ["new_session", "switch_session", "switch_model", "toggle_yolo", "toggle_help", "init", "quit"],
        )
        self.assertNotIn("custom_", " ".join(got["names"]))
        self.assertEqual(got["aliases"], {"new_session": ["clear"], "quit": ["exit"]})

    def test_parse_commands_fails_when_nothing_matches(self):
        with self.assertRaises(ValueError):
            self.ex.parse_commands("package dialog\n")

    def test_parse_agents_maps_the_keys_to_their_string_constants(self):
        # Arrange
        text = helpers.read("crush", "config.go")
        # Act / Assert
        self.assertEqual(self.ex.parse_agents(text), ["coder", "task", "plan"])

    def test_parse_agents_fails_on_a_key_that_is_not_a_constant(self):
        text = helpers.read("crush", "config.go").replace("AgentPlan: {", "agentPlan: {")
        with self.assertRaises(ValueError):
            self.ex.parse_agents(text)

    def test_parse_agents_fails_when_setupagents_is_gone(self):
        with self.assertRaises(ValueError):
            self.ex.parse_agents("package config\n")

    def test_extract_reads_embedded_skill_names_from_front_matter(self):
        # Arrange
        ctx = helpers.context(
            "charmbracelet/crush",
            "v0.98.1",
            files={
                self.ex.COMMANDS: helpers.read("crush", "commands.go"),
                self.ex.CONFIG: helpers.read("crush", "config.go"),
                SK + "jq/SKILL.md": "---\nname: jq\ndescription: x\n---\n",
                SK + "crush-config/SKILL.md": "---\nname: crush-config\n---\n",
            },
            tree=[SK + "jq/SKILL.md", SK + "crush-config/SKILL.md", SK + "jq/references/SKILL.md"],
        )
        # Act
        got = self.ex.extract(ctx)
        # Assert
        self.assertEqual(got["skill"]["names"], ["crush-config", "jq"])


if __name__ == "__main__":
    unittest.main()
