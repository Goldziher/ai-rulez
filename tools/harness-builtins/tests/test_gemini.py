import re
import unittest

from tests import helpers

CMD_DIR = "packages/cli/src/ui/commands/"
AGENT_DIR = "packages/core/src/agents/"
FIXTURE_COMMANDS = {
    "clearCommand": "commands/clearCommand.ts",
    "quitCommand": "commands/quitCommand.ts",
    "corgiCommand": "commands/corgiCommand.ts",
    "compressCommand": "commands/compressCommand.ts",
    "authCommand": "commands/authCommand.ts",
    "extensionsCommand": "commands/extensionsCommand.ts",
}


def stub(ident):
    return f"export const {ident}: SlashCommand = {{\n  name: 'stub-{ident}',\n}};\n"


class GeminiTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("gemini")
        self.loader = helpers.read("gemini", "BuiltinCommandLoader.ts")

    def command_files(self):
        """Fixture modules for the commands we saved, a stub for every other imported command."""
        files = {self.ex.LOADER: self.loader}
        for ident, module in self.ex.parse_imports(self.loader).items():
            if not ident.endswith("Command") or not module.startswith("."):
                continue
            base = module.rsplit("/", 1)[-1].removesuffix(".js")
            if ident in FIXTURE_COMMANDS:
                files[CMD_DIR + base + ".ts"] = helpers.read("gemini", FIXTURE_COMMANDS[ident])
            elif ident == "directoryCommand":
                files[CMD_DIR + base + ".tsx"] = helpers.read("gemini", "commands", "directoryCommand.tsx")
            else:  # several imports may share a module (chatCommand, debugCommand)
                files[CMD_DIR + base + ".ts"] = files.get(CMD_DIR + base + ".ts", "") + stub(ident)
        return files

    def test_commands_lists_every_loaded_command_with_its_own_name_and_aliases(self):
        # Arrange
        ctx = helpers.context("google-gemini/gemini-cli", "v0.64.0", files=self.command_files())
        # Act
        got = self.ex.commands(ctx, self.ex.LOADER)
        # Assert
        self.assertEqual(len(got["names"]), 47)
        self.assertEqual(len(set(got["names"])), 47)
        self.assertIn("clear", got["names"])
        self.assertIn("extensions", got["names"])  # exported as a function, built by the admin-disabled branch too
        self.assertIn("directory", got["names"])  # a .tsx module
        self.assertEqual(got["aliases"]["compress"], ["summarize", "compact"])
        self.assertEqual(got["aliases"]["quit"], ["exit"])
        self.assertEqual(got["aliases"]["directory"], ["dir"])

    def test_commands_keeps_hidden_commands(self):
        # Arrange
        ctx = helpers.context("o/r", "t", files=self.command_files())
        # Act
        got = self.ex.commands(ctx, self.ex.LOADER)
        # Assert
        self.assertIn("corgi", got["names"])

    def test_commands_ignores_subcommand_aliases_and_helper_imports(self):
        # Arrange
        ctx = helpers.context("o/r", "t", files=self.command_files())
        # Act
        got = self.ex.commands(ctx, self.ex.LOADER)
        # Assert
        self.assertNotIn("auth", got["aliases"])  # login/logout are altNames of its subcommands
        for name in ("login", "logout", "list", "debug", "CommandKind", "CommandContext"):
            self.assertNotIn(name, got["names"])

    def test_declared_name_reads_the_first_name_after_the_declaration(self):
        # Arrange
        text = helpers.read("gemini", "commands", "authCommand.ts")
        # Act
        name, aliases = self.ex.declared_name(text, "authCommand")
        # Assert
        self.assertEqual((name, aliases), ("auth", []))

    def test_array_body_fails_when_the_loader_has_no_definitions(self):
        with self.assertRaises(ValueError):
            self.ex.array_body("export class X {}", "allDefinitions")

    def test_commands_fails_when_a_command_module_has_no_declaration(self):
        # Arrange
        files = self.command_files()
        files[CMD_DIR + "clearCommand.ts"] = "export const somethingElse = {};\n"
        ctx = helpers.context("o/r", "t", files=files)
        # Act / Assert
        with self.assertRaisesRegex(ValueError, "clearCommand"):
            self.ex.commands(ctx, self.ex.LOADER)

    def test_skills_reads_front_matter_names_of_top_level_skill_files(self):
        # Arrange
        pre = self.ex.SKILLS
        tree = [
            pre + "skill-creator/SKILL.md",
            pre + "skill-creator/scripts/init_skill.cjs",
            pre + "antigravity-support/SKILL.md",
            pre + "skill-creator/references/SKILL.md",
        ]
        ctx = helpers.context(
            "o/r",
            "t",
            files={
                pre + "skill-creator/SKILL.md": "---\nname: skill-creator\ndescription: x\n---\n",
                pre + "antigravity-support/SKILL.md": "no front matter\n",
            },
            tree=tree,
        )
        # Act
        got = self.ex.skills(ctx, pre)
        # Assert
        self.assertEqual(got["names"], ["antigravity-support", "skill-creator"])  # tree order

    def test_skills_fails_when_there_are_none(self):
        ctx = helpers.context("o/r", "t", tree=["packages/other/file.ts"])
        with self.assertRaises(ValueError):
            self.ex.skills(ctx, self.ex.SKILLS)

    def test_agents_resolves_names_and_constants_of_the_registered_factories(self):
        # Arrange
        files = {self.ex.REGISTRY: helpers.read("gemini", "agents", "registry.ts")}
        for module, fixture in (
            ("codebase-investigator", "codebase-investigator.ts"),
            ("cli-help-agent", "cli-help-agent.ts"),
            ("generalist-agent", "generalist-agent.ts"),
            ("browser/browserAgentDefinition", "browserAgentDefinition.ts"),
        ):
            files[AGENT_DIR + module + ".ts"] = helpers.read("gemini", "agents", fixture)
        ctx = helpers.context("o/r", "t", files=files)
        # Act
        got = self.ex.agents(ctx, self.ex.REGISTRY)
        # Assert
        self.assertEqual(got["names"], ["codebase_investigator", "cli_help", "generalist", "browser_agent"])

    def test_builtin_agent_calls_fails_without_the_loader_function(self):
        with self.assertRaises(ValueError):
            self.ex.builtin_agent_calls("class AgentRegistry {}")

    def test_strip_comments_keeps_globs_in_strings(self):
        text = "const a = '**/*.ts'; // x\n/* block\n   comment */\nconst b = 1;\n"
        self.assertIn("'**/*.ts'", self.ex.strip_comments(text))
        self.assertNotIn("block", self.ex.strip_comments(text))
        self.assertTrue(re.search(r"const b = 1;", self.ex.strip_comments(text)))


if __name__ == "__main__":
    unittest.main()
