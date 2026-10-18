import unittest

from tests import helpers


class OmpTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("omp")

    def test_parse_commands_reads_top_level_specs_only(self):
        # Arrange
        text = helpers.read("omp", "builtin-control.ts")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertEqual(got, {"force": ["force:"], "live": [], "todo": [], "quit": ["q"]})

    def test_parse_embedded_agents_reads_inline_names_and_skips_comments(self):
        # Arrange
        text = helpers.read("omp", "agents.ts")
        # Act
        got = self.ex.parse_embedded_agents(text)
        # Assert
        self.assertEqual(got, [("scout.md", None), ("task.md", "task")])

    def test_parse_embedded_agents_fails_when_the_table_is_gone(self):
        # Arrange / Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_embedded_agents("export const x = [];")

    def test_extract_reads_command_files_and_agent_frontmatter(self):
        # Arrange
        d = self.ex.SLASH_DIR
        ctx = helpers.context(
            "can1357/oh-my-pi",
            "v1",
            files={
                d + "builtin-control.ts": helpers.read("omp", "builtin-control.ts"),
                d + "builtin-registry.ts": '\t\tname: "registry-only",\n',
                d + "helpers/builtin-x.ts": '\t\tname: "nested-dir",\n',
                self.ex.AGENTS: helpers.read("omp", "agents.ts"),
                self.ex.AGENT_PROMPTS + "scout.md": helpers.read("omp", "scout.md"),
            },
            tree=[d + "builtin-control.ts", d + "builtin-registry.ts", d + "helpers/builtin-x.ts", d + "types.ts"],
        )
        # Act
        got = self.ex.extract(ctx)
        # Assert
        self.assertEqual(got["command"]["names"], ["force", "live", "todo", "quit"])
        self.assertEqual(got["command"]["aliases"], {"force": ["force:"], "quit": ["q"]})
        self.assertEqual(got["agent"]["names"], ["scout", "task"])


if __name__ == "__main__":
    unittest.main()
