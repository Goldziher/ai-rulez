import unittest

from tests import helpers


class XumTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("xum")

    def test_parse_commands_expands_spreads_and_skips_sub_commands_and_comments(self):
        # Arrange
        text = helpers.read("xum", "registry.ts")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertEqual(got, ["clear", "plan", "fast", "ultrafast"])

    def test_parse_commands_fails_on_an_unresolvable_entry(self):
        # Arrange
        text = (
            "export const SLASH_COMMAND_DEFINITIONS: readonly SlashCommandDefinition[] = [\n  mysteryDefinition,\n];\n"
        )
        # Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_commands(text)

    def test_parse_agent_ids_skips_comments(self):
        # Arrange
        text = helpers.read("xum", "builtInAgentDefinitions.ts")
        # Act
        got = self.ex.parse_agent_ids(text)
        # Assert
        self.assertEqual(got, ["exec", "plan", "name_workspace"])

    def test_parse_agent_ids_fails_when_the_list_is_gone(self):
        # Arrange / Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_agent_ids("export const x = 1;")

    def test_extract_names_skills_by_file_stem(self):
        # Arrange
        s = self.ex.SKILLS
        ctx = helpers.context(
            "coder/mux",
            "abc",
            files={
                self.ex.REGISTRY: helpers.read("xum", "registry.ts"),
                self.ex.AGENT_LIST: helpers.read("xum", "builtInAgentDefinitions.ts"),
            },
            tree=[s + "init.md", s + "loop.md", s + "loop/workflow.js", s + "loop/notes.md"],
        )
        # Act
        got = self.ex.extract(ctx)
        # Assert
        self.assertEqual(got["skill"]["names"], ["init", "loop"])
        self.assertEqual(got["command"]["names"], ["clear", "plan", "fast", "ultrafast"])
        self.assertEqual(got["agent"]["names"], ["exec", "plan", "name_workspace"])


if __name__ == "__main__":
    unittest.main()
