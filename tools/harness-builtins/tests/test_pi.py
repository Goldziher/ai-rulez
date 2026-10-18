import unittest

from tests import helpers

EXT = "packages/coding-agent/src/extensions/"


class PiTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("pi")

    def test_parse_builtin_commands_reads_every_registry_entry(self):
        # Arrange
        text = helpers.read("pi", "slash-commands.ts")
        # Act
        got = self.ex.parse_builtin_commands(text)
        # Assert
        self.assertEqual(len(got), 24)
        self.assertEqual(got[:2], ["settings", "model"])
        self.assertIn("scoped-models", got)
        self.assertIn("quit", got, "an entry whose description is a template literal")

    def test_parse_builtin_commands_fails_when_the_registry_moved(self):
        with self.assertRaises(ValueError):
            self.ex.parse_builtin_commands("export const OTHER = []\n")

    def test_parse_builtin_extensions_lists_the_modules_of_the_registered_factories(self):
        # Arrange
        text = helpers.read("pi", "extensions-index.ts")
        # Act
        got = self.ex.parse_builtin_extensions(text)
        # Assert
        self.assertEqual(got, ["./llama/index.ts", "./codemode/index.ts", "./tool-search/index.ts", "./mcp/index.ts"])

    def test_parse_builtin_extensions_fails_on_a_factory_without_import(self):
        text = "export const builtInExtensions = [\n\t{ name: 'x', factory: missing },\n];\n"
        with self.assertRaises(ValueError):
            self.ex.parse_builtin_extensions(text)

    def test_parse_registered_commands_ignores_commented_out_registrations(self):
        text = (
            '\t// pi.registerCommand("old", {})\n\tpi.registerCommand("new", {})\n\tpi.registerCommand(\'two\', {})\n'
        )
        self.assertEqual(self.ex.parse_registered_commands(text), ["new", "two"])

    def test_extract_adds_the_commands_of_the_builtin_extensions(self):
        # Arrange
        ctx = helpers.context(
            "earendil-works/pi",
            "v1.3.0",
            files={
                self.ex.SLASH: helpers.read("pi", "slash-commands.ts"),
                self.ex.EXTENSIONS: helpers.read("pi", "extensions-index.ts"),
                EXT + "llama/index.ts": helpers.read("pi", "llama.ts"),
                EXT + "mcp/index.ts": helpers.read("pi", "mcp.ts"),
                EXT + "codemode/index.ts": "export default function () {}\n",
                EXT + "tool-search/index.ts": "export default function () {}\n",
            },
        )
        # Act
        got = self.ex.extract(ctx)["command"]["names"]
        # Assert
        self.assertEqual(len(got), 26)
        self.assertEqual(got[-2:], ["llama", "mcp"])


if __name__ == "__main__":
    unittest.main()
