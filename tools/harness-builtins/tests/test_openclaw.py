import unittest

from tests import helpers


class OpenclawTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("openclaw")

    def test_parse_registry_reads_keys_and_aliases_and_skips_comments(self):
        # Arrange
        text = helpers.read("openclaw", "commands-registry.shared.ts")
        # Act
        got = self.ex.parse_registry(text)
        # Assert
        self.assertEqual(got["names"], ["help", "dashboard", "btw", "export-session", "reset", "think"])
        self.assertEqual(got["aliases"], {"btw": ["side"], "export-session": ["export"], "think": ["thinking", "t"]})

    def test_parse_registry_fails_when_the_registry_is_gone(self):
        # Arrange / Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_registry("export const x = 1;")

    def test_parse_plugin_commands_reads_the_first_column(self):
        # Arrange
        text = helpers.read("openclaw", "slash-commands.md")
        # Act
        got = self.ex.parse_plugin_commands(text)
        # Assert
        self.assertEqual(got, ["dreaming", "pair", "voice", "codex"])

    def test_parse_plugin_commands_fails_without_the_section(self):
        # Arrange / Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_plugin_commands("# Other\n")

    def test_parse_reserved_reads_the_sentence(self):
        # Arrange
        text = helpers.read("openclaw", "slash-commands.md")
        # Act / Assert
        self.assertEqual(self.ex.parse_reserved(text), ["dashboard"])
        with self.assertRaises(ValueError):
            self.ex.parse_reserved("nothing reserved")

    def test_extract_combines_commands_and_skills(self):
        # Arrange
        ctx = helpers.context(
            "openclaw/openclaw",
            "v1",
            files={
                self.ex.REGISTRY: helpers.read("openclaw", "commands-registry.shared.ts"),
                self.ex.DOCS: helpers.read("openclaw", "slash-commands.md"),
                "skills/github/SKILL.md": "---\nname: github\n---\n",
                "skills/tmux/SKILL.md": "no front matter\n",
            },
            tree=["skills/github/SKILL.md", "skills/tmux/SKILL.md", "skills/github/refs/SKILL.md", "skills/README.md"],
        )
        # Act
        got = self.ex.extract(ctx)
        # Assert
        self.assertEqual(got["command"]["names"][-4:], ["dreaming", "pair", "voice", "codex"])
        self.assertIn("dashboard", got["command"]["names"])
        self.assertEqual(got["reserved"], {"names": ["dashboard"]})
        self.assertEqual(got["skill"], {"names": ["github", "tmux"]})


if __name__ == "__main__":
    unittest.main()
