import unittest

from tests import helpers


class ClineTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("cline")

    def test_entry_names_reads_the_tui_commands_including_the_hidden_one(self):
        # Arrange
        text = helpers.read("cline", "slash-command-registry.ts")
        # Act
        got = self.ex.entry_names(text, "const TUI_LOCAL_COMMANDS")
        # Assert
        self.assertEqual(got[:3], ["settings", "config", "model"])
        self.assertIn("config", got)
        self.assertEqual(got[-2:], ["help", "quit"])
        self.assertEqual(len(got), 15)

    def test_entry_names_ignores_commented_out_entries(self):
        # Arrange
        text = 'const LIST: Array<{ name: string }> = [\n\t// { name: "old" },\n\t{ name: "kept" },\n\t/* { name: "gone" }, */\n]\n'
        # Act
        got = self.ex.entry_names(text, "const LIST")
        # Assert
        self.assertEqual(got, ["kept"])

    def test_compaction_commands_reads_the_intercepted_spellings(self):
        # Arrange
        text = helpers.read("cline", "useMessageHandlers.ts")
        # Act
        got = self.ex.compaction_commands(text)
        # Assert
        self.assertEqual(got, ["compact", "smol", "newtask"])

    def test_builtin_names_come_from_the_exported_array_not_the_prompt(self):
        # Arrange
        text = helpers.read("cline", "builtin-slash-commands.ts")
        # Act
        got = self.ex.entry_names(text, "export const BUILTIN_SLASH_COMMANDS")
        # Assert
        self.assertEqual(got, ["deep-planning"])

    def test_extract_reads_the_three_files(self):
        # Arrange
        ctx = helpers.context(
            "cline/cline",
            "v1",
            files={
                self.ex.TUI: helpers.read("cline", "slash-command-registry.ts"),
                self.ex.WEBVIEW: helpers.read("cline", "useMessageHandlers.ts"),
                self.ex.BUILTIN: helpers.read("cline", "builtin-slash-commands.ts"),
            },
        )
        # Act
        got = self.ex.extract(ctx)
        # Assert
        self.assertEqual(sorted(got), ["command_builtin", "command_tui", "command_webview"])
        self.assertIn("history", got["command_tui"]["names"])

    def test_a_changed_format_is_an_error_not_an_empty_result(self):
        # Arrange
        text = "export const OTHER = []\n"
        # Act / Assert
        with self.assertRaises(ValueError):
            self.ex.entry_names(text, "const TUI_LOCAL_COMMANDS")
        with self.assertRaises(ValueError):
            self.ex.compaction_commands("function other() {}")
        with self.assertRaises(ValueError):
            self.ex.entry_names("const TUI_LOCAL_COMMANDS = []", "const TUI_LOCAL_COMMANDS")


if __name__ == "__main__":
    unittest.main()
