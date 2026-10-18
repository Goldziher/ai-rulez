import unittest

from tests import helpers


class MimocodeTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("mimocode")

    def test_parse_defaults_reads_assigned_commands_only(self):
        # Arrange
        text = helpers.read("mimocode", "command-index.ts")
        # Act
        got = self.ex.parse_defaults(text)
        # Assert
        self.assertEqual(got, ["init", "loops", "goal"])

    def test_parse_defaults_fails_when_the_table_is_gone(self):
        # Arrange / Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_defaults("export const Other = {} as const")

    def test_parse_slash_reads_names_and_aliases_and_skips_comments(self):
        # Arrange
        text = helpers.read("mimocode", "app.tsx")
        # Act
        got = self.ex.parse_slash(text)
        # Assert
        self.assertEqual(got, {"sessions": ["resume", "continue"], "workflows": []})

    def test_parse_slash_fails_on_an_entry_it_cannot_read(self):
        # Arrange
        text = 'slash: { name: "a" },\nslash: { ...spread },\n'
        # Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_slash(text)

    def test_parse_agents_skips_opt_in_entries_and_comments(self):
        # Arrange
        text = helpers.read("mimocode", "agent.ts")
        # Act
        got = self.ex.parse_agents(text)
        # Assert
        self.assertEqual(got, ["build", "title"])

    def test_extract_combines_the_sources(self):
        # Arrange
        s = self.ex.SKILLS
        ctx = helpers.context(
            "XiaomiMiMo/MiMo-Code",
            "abc",
            files={
                self.ex.COMMANDS: helpers.read("mimocode", "command-index.ts"),
                self.ex.TUI + "app.tsx": helpers.read("mimocode", "app.tsx"),
                self.ex.TUI + "component/prompt/index.tsx": "no slash entries\n",
                self.ex.TUI + "routes/session/index.tsx": 'slash: {\n name: "recover",\n},\n',
                self.ex.AGENTS: helpers.read("mimocode", "agent.ts"),
                s + "pdf-official/SKILL.md": '---\nname: "pdf-official"\n---\n',
                s + "loop/SKILL.md": "no front matter\n",
            },
            tree=[s + "pdf-official/SKILL.md", s + "loop/SKILL.md", s + "loop/refs/SKILL.md"],
        )
        # Act
        got = self.ex.extract(ctx)
        # Assert
        self.assertEqual(got["command"]["names"], ["init", "loops", "goal"])
        self.assertEqual(got["tui"]["names"], ["sessions", "workflows", "recover"])
        self.assertEqual(got["tui"]["aliases"], {"sessions": ["resume", "continue"]})
        self.assertEqual(got["skill"]["names"], ["loop", "pdf-official"])
        self.assertEqual(got["agent"]["names"], ["build", "title"])


if __name__ == "__main__":
    unittest.main()
