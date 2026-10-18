import unittest

from tests import helpers


class VibeTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("vibe")

    def test_parse_commands_reads_keys_and_aliases_without_the_slash(self):
        # Arrange
        text = helpers.read("vibe", "commands.py")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertEqual(len(got["names"]), 34)
        self.assertIn("paste-image", got["names"])
        self.assertEqual(got["aliases"]["clear"], ["new"])
        self.assertEqual(got["aliases"]["exit"], [":q", ":quit", "quit"])
        self.assertEqual(got["aliases"]["mcp"], ["connectors"])
        self.assertNotIn("help", got["aliases"])

    def test_parse_commands_keeps_commands_that_are_only_available_in_some_contexts(self):
        # Arrange
        text = helpers.read("vibe", "commands.py")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        for name in ("skills", "plugins", "reload-plugins", "todo"):
            self.assertIn(name, got["names"])

    def test_skill_modules_and_names(self):
        # Arrange
        init = helpers.read("vibe", "builtins_init.py")
        # Act
        modules = self.ex.skill_modules(init)
        # Assert
        self.assertEqual(modules, ["vibe/core/skills/builtins/skill_creator.py", "vibe/core/skills/builtins/vibe.py"])
        self.assertEqual(self.ex.skill_name(helpers.read("vibe", "skill_vibe.py")), "vibe")
        self.assertEqual(self.ex.skill_name(helpers.read("vibe", "skill_skill_creator.py")), "skill-creator")

    def test_parse_agents_resolves_the_enum_members(self):
        # Arrange
        text = helpers.read("vibe", "agents_models.py")
        # Act
        got = self.ex.parse_agents(text)
        # Assert
        self.assertEqual(got, ["ask", "plan", "accept-edits", "smart-approve", "auto-approve", "explore", "lean"])

    def test_extract_reads_the_files_of_the_pin(self):
        # Arrange
        ctx = helpers.context(
            "mistralai/mistral-vibe",
            "v1",
            files={
                self.ex.COMMANDS: helpers.read("vibe", "commands.py"),
                self.ex.SKILLS_INIT: helpers.read("vibe", "builtins_init.py"),
                "vibe/core/skills/builtins/vibe.py": helpers.read("vibe", "skill_vibe.py"),
                "vibe/core/skills/builtins/skill_creator.py": helpers.read("vibe", "skill_skill_creator.py"),
                self.ex.AGENTS: helpers.read("vibe", "agents_models.py"),
            },
        )
        # Act
        got = self.ex.extract(ctx)
        # Assert
        self.assertEqual(got["skill"]["names"], ["skill-creator", "vibe"])
        self.assertIn("lean", got["agent"]["names"])

    def test_a_changed_format_is_an_error_not_an_empty_result(self):
        # Arrange / Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_commands("class CommandRegistry:\n    pass\n")
        with self.assertRaises(ValueError):
            self.ex.parse_commands("def (:\n")
        with self.assertRaises(ValueError):
            self.ex.skill_modules("X = 1\n")
        with self.assertRaises(ValueError):
            self.ex.skill_name("X = 1\n")
        with self.assertRaises(ValueError):
            self.ex.parse_agents("X = 1\n")


if __name__ == "__main__":
    unittest.main()
