import unittest

from tests import helpers

TUI = "packages/tui/src/"


class OpencodeTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("opencode")

    def test_parse_defaults_reads_the_default_table_not_user_config_entries(self):
        # Arrange
        text = helpers.read("opencode", "command_index.ts")
        # Act
        got = self.ex.parse_defaults(text)
        # Assert
        self.assertEqual(got, ["init", "review"])

    def test_parse_defaults_fails_when_the_table_is_gone(self):
        with self.assertRaises(ValueError):
            self.ex.parse_defaults("const commands = {}\ncommands[Default.INIT] = {}\n")

    def test_parse_defaults_fails_on_a_key_missing_from_the_table(self):
        text = 'export const Default = { INIT: "init" } as const\ncommands[Default.OTHER] = {}\n'
        with self.assertRaises(ValueError):
            self.ex.parse_defaults(text)

    def test_parse_slashes_reads_slash_name_aliases_and_nested_entries(self):
        # Arrange
        text = helpers.read("opencode", "app.tsx")
        # Act
        got = dict(self.ex.parse_slashes(text))
        # Assert
        self.assertEqual(got["sessions"], ["resume", "continue"])
        self.assertEqual(got["new"], ["clear"])
        self.assertEqual(got["models"], ["mo"], "an alias after a comment line")
        self.assertEqual(got["org"], ["orgs", "switch-org"], "an entry nested in a conditional spread")
        self.assertEqual(got["status"], [])

    def test_parse_slashes_reads_the_slash_object_form_and_skips_the_forwarding_expression(self):
        # Arrange
        text = helpers.read("opencode", "session.tsx")
        # Act
        got = dict(self.ex.parse_slashes(text))
        # Assert
        self.assertEqual(got["compact"], ["summarize"])
        self.assertEqual(got["timestamps"], ["toggle-timestamps"])
        self.assertEqual(got["thinking"], ["toggle-thinking"])
        self.assertNotIn("slash", got, '`slashName: "slash" in command ? ...` is not a name')

    def test_parse_slashes_ignores_commented_out_entries(self):
        text = '  // slashName: "old",\n  slashName: "kept",\n'
        self.assertEqual(self.ex.parse_slashes(text), [("kept", [])])

    def test_slash_commands_scans_every_ts_file_and_merges_aliases(self):
        # Arrange
        tree = [
            TUI + "app.tsx",
            TUI + "routes/session/index.tsx",
            TUI + "other.ts",
            TUI + "app.test.tsx",
            TUI + "x.css",
        ]
        ctx = helpers.context(
            "o/r",
            "t",
            files={
                TUI + "app.tsx": helpers.read("opencode", "app.tsx"),
                TUI + "routes/session/index.tsx": helpers.read("opencode", "session.tsx"),
                TUI + "other.ts": 'export const x = { slashName: "sessions", slashAliases: ["switch"] }\n',
            },
            tree=tree,
        )
        # Act
        got = self.ex.slash_commands(ctx, TUI)
        # Assert
        self.assertIn("compact", got["names"])
        self.assertEqual(got["names"].count("sessions"), 1)
        self.assertEqual(got["aliases"]["sessions"], ["continue", "resume", "switch"])

    def test_slash_commands_fails_when_no_file_has_a_slash_name(self):
        ctx = helpers.context("o/r", "t", files={TUI + "a.ts": "export {}\n"}, tree=[TUI + "a.ts"])
        with self.assertRaises(ValueError):
            self.ex.slash_commands(ctx, TUI)

    def test_parse_builtin_skills_resolves_the_name_constant(self):
        # Arrange
        text = helpers.read("opencode", "skill_index.ts")
        # Act / Assert
        self.assertEqual(self.ex.parse_builtin_skills(text), ["customize-opencode"])

    def test_parse_native_agents_stops_before_the_users_agent_config(self):
        # Arrange
        text = helpers.read("opencode", "agent.ts")
        # Act
        got = self.ex.parse_native_agents(text)
        # Assert
        self.assertEqual(got, ["build", "plan", "summary"])

    def test_parse_native_agents_fails_when_the_record_moved(self):
        with self.assertRaises(ValueError):
            self.ex.parse_native_agents("const agents = {}\n")


if __name__ == "__main__":
    unittest.main()
