import unittest

from tests import helpers

CMD_DIR = "packages/cli/src/ui/commands/"
FIXTURES = {
    "aboutCommand": ("aboutCommand.ts", "commands/aboutCommand.ts"),
    "helpCommand": ("helpCommand.ts", "commands/helpCommand.ts"),
    "clearCommand": ("clearCommand.ts", "commands/clearCommand.ts"),
    "advisorCommand": ("advisor-command.ts", "commands/advisor-command.ts"),
    "ideCommand": ("ideCommand.ts", "commands/ideCommand.ts"),
}


class QwenTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("qwen")
        self.loader = helpers.read("qwen", "BuiltinCommandLoader.ts")

    def files(self):
        files = {self.ex.LOADER: self.loader, self.ex.AGENTS: helpers.read("qwen", "builtin-agents.ts")}
        for ident, module in self.ex.gemini.parse_imports(self.loader).items():
            if not ident.endswith("Command") or not module.startswith("."):
                continue
            base = module.rsplit("/", 1)[-1].removesuffix(".js")
            if ident in FIXTURES:
                path, fixture = FIXTURES[ident]
                files[CMD_DIR + path.removesuffix(".ts") + ".ts"] = helpers.read("qwen", fixture)
            else:
                files[CMD_DIR + base + ".ts"] = (
                    f"export const {ident}: SlashCommand = {{\n  name: 'stub-{ident}',\n}};\n"
                )
        return files

    def test_extract_reads_commands_aliases_skills_and_agents(self):
        # Arrange
        sk = self.ex.SKILLS
        files = self.files()
        files[sk + "loop/SKILL.md"] = "---\nname: loop\n---\n"
        ctx = helpers.context("QwenLM/qwen-code", "v0.25.2", files=files, tree=[sk + "loop/SKILL.md", sk + "loop/x.md"])
        # Act
        got = self.ex.extract(ctx)
        # Assert
        cmds = got["command"]
        self.assertEqual(len(cmds["names"]), 71)
        self.assertIn("status", cmds["names"])
        self.assertNotIn("about", cmds["names"])
        self.assertEqual(cmds["aliases"]["status"], ["about"])
        self.assertEqual(cmds["aliases"]["help"], ["?"])
        self.assertEqual(cmds["aliases"]["clear"], ["reset", "new"])
        self.assertEqual(got["skill"]["names"], ["loop"])
        self.assertEqual(
            got["agent"]["names"],
            ["general-purpose", "Explore", "statusline-setup", "review-agent", "claude-code", "codex"],
        )

    def test_extract_includes_the_command_built_outside_the_definitions_array(self):
        # Arrange
        files = self.files()
        files[self.ex.SKILLS + "a/SKILL.md"] = "---\nname: a\n---\n"
        ctx = helpers.context("o/r", "t", files=files, tree=[self.ex.SKILLS + "a/SKILL.md"])
        # Act
        got = self.ex.extract(ctx)["command"]
        # Assert
        self.assertIn("ide", got["names"])  # `resolvedIdeCommand = await ideCommand()`, not in the array

    def test_commands_that_are_gated_by_a_flag_are_still_listed(self):
        # Arrange: workflowsCommand is in the array only behind isWorkflowsEnabled()
        self.assertIn("this.config?.isWorkflowsEnabled() ? workflowsCommand : null", self.loader)
        files = self.files()
        files[self.ex.SKILLS + "a/SKILL.md"] = "---\nname: a\n---\n"
        ctx = helpers.context("o/r", "t", files=files, tree=[self.ex.SKILLS + "a/SKILL.md"])
        # Act
        got = self.ex.extract(ctx)["command"]
        # Assert
        self.assertIn("stub-workflowsCommand", got["names"])

    def test_parse_agents_resolves_constants_and_ignores_commented_out_entries(self):
        # Arrange
        text = helpers.read("qwen", "builtin-agents.ts").replace(
            "  ];\n", "    // {\n    //   name: 'disabled',\n    // },\n  ];\n"
        )
        # Act
        got = self.ex.parse_agents(text)
        # Assert
        self.assertEqual(got[0], "general-purpose")
        self.assertEqual(got[3], "review-agent")
        self.assertNotIn("disabled", got)

    def test_parse_agents_fails_when_the_array_moved(self):
        with self.assertRaises(ValueError):
            self.ex.parse_agents("export class BuiltinAgentRegistry {}\n")


if __name__ == "__main__":
    unittest.main()
