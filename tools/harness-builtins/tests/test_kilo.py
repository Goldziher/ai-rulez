import unittest

from tests import helpers

TUI = "packages/tui/src/"
KILO = "packages/opencode/src/kilocode/"


class KiloTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("kilo")

    def test_parse_registered_reads_default_goal_and_quoted_names_but_not_config_entries(self):
        # Arrange
        text = helpers.read("kilo", "command_index.ts")
        # Act
        got = self.ex.parse_registered(text)
        # Assert
        self.assertEqual(got, ["init", "review", "goal", "resume-claude", "resume-codex"])

    def test_parse_reserved_reads_the_names_a_user_command_may_not_take(self):
        # Arrange
        text = helpers.read("kilo", "reserved.ts")
        # Act / Assert
        self.assertEqual(self.ex.parse_reserved(text), ["goal"])

    def test_parse_reserved_fails_when_the_function_changed(self):
        with self.assertRaises(ValueError):
            self.ex.parse_reserved("export function reserved(name: string) { return false }\n")

    def test_extract_splits_user_overridable_and_reserved_commands(self):
        # Arrange
        files = {
            self.ex.COMMANDS: helpers.read("kilo", "command_index.ts"),
            self.ex.RESERVED: helpers.read("kilo", "reserved.ts"),
            self.ex.SKILLS: helpers.read("kilo", "builtin.ts"),
            self.ex.AGENTS: helpers.read("kilo", "agent.ts"),
            self.ex.PATCH: helpers.read("kilo", "agent_index.ts"),
            TUI + "a.tsx": 'x = { slashName: "sessions", slashAliases: ["resume"] }\n',
            KILO + "kilo-commands.tsx": helpers.read("kilo", "kilo-commands.tsx"),
        }
        tree = [TUI + "a.tsx", KILO + "kilo-commands.tsx"]
        ctx = helpers.context("Kilo-Org/kilocode", "v7.8.14", files=files, tree=tree)
        # Act
        got = self.ex.extract(ctx)
        # Assert
        self.assertEqual(got["command_default"]["names"], ["init", "review", "resume-claude", "resume-codex"])
        self.assertEqual(got["command_reserved"]["names"], ["goal"])
        tui = got["command_tui"]
        self.assertIn("sessions", tui["names"])
        self.assertIn("indexing", tui["names"], "an entry behind `...(indexing() ? [...] : [])`")
        self.assertEqual(tui["aliases"]["teams"], ["org", "orgs", "team"])
        self.assertEqual(got["skill"]["names"], ["kilo-config"])

    def test_extract_fails_when_a_reserved_name_is_not_a_registered_command(self):
        # Arrange
        files = {
            self.ex.COMMANDS: helpers.read("kilo", "command_index.ts").replace("commands.goal", "commands.other"),
            self.ex.RESERVED: helpers.read("kilo", "reserved.ts"),
        }
        ctx = helpers.context("o/r", "t", files=files, tree=[])
        # Act / Assert
        with self.assertRaisesRegex(ValueError, "goal"):
            self.ex.extract(ctx)

    def test_apply_patch_renames_build_to_code_and_adds_the_agents_patchagents_creates(self):
        # Arrange
        base = self.ex.oc.parse_native_agents(helpers.read("kilo", "agent.ts"))
        # Act
        got = self.ex.apply_patch(base, helpers.read("kilo", "agent_index.ts"))
        # Assert
        self.assertEqual(base, ["build", "scout", "compaction"], "scout is behind a flag but is listed")
        self.assertEqual(got, ["code", "scout", "compaction", "debug", "orchestrator", "ask"])

    def test_apply_patch_fails_without_patchagents(self):
        with self.assertRaises(ValueError):
            self.ex.apply_patch(["build"], "export const x = 1\n")

    def test_parse_builtin_skills_fails_when_the_array_is_gone(self):
        with self.assertRaises(ValueError):
            self.ex.parse_builtin_skills("export const OTHER = []\n")


if __name__ == "__main__":
    unittest.main()
