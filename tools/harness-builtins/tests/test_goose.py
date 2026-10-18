import unittest

from tests import helpers


class GooseTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("goose")

    def test_parse_agent_commands_reads_the_table_and_the_deprecated_trigger(self):
        # Arrange
        text = helpers.read("goose", "execute_commands.rs")
        # Act
        got = self.ex.parse_agent_commands(text)
        # Assert
        self.assertEqual(
            got,
            ["prompts", "prompt", "compact", "clear", "skills", "doctor", "goal", "grind", "status", "summarize"],
        )

    def test_parse_agent_commands_skips_commented_out_entries(self):
        # Arrange
        text = (
            'pub const COMPACT_TRIGGERS: &[&str] = &["/compact"];\n'
            'static COMMANDS: &[CommandDef] = &[\n    // CommandDef { name: "old", description: "x" },\n'
            '    CommandDef { name: "kept", description: "y" },\n];\n'
        )
        # Act
        got = self.ex.parse_agent_commands(text)
        # Assert
        self.assertEqual(got, ["kept", "compact"])

    def test_parse_session_commands_reads_consts_arms_and_aliases(self):
        # Arrange
        text = helpers.read("goose", "input.rs")
        # Act
        got = self.ex.parse_session_commands(text)
        # Assert
        for name in ("exit", "help", "t", "r", "extension", "builtin", "mode", "model", "edit", "new", "prompts"):
            self.assertIn(name, got["names"])
        self.assertNotIn("quit", got["names"])
        self.assertEqual(got["aliases"], {"exit": ["quit"], "help": ["?"]})

    def test_extract_reads_the_front_matter_name_of_each_skill_file(self):
        # Arrange
        skills = self.ex.SKILLS
        ctx = helpers.context(
            "block/goose",
            "v1",
            files={
                self.ex.EXECUTE: helpers.read("goose", "execute_commands.rs"),
                self.ex.INPUT: helpers.read("goose", "input.rs"),
                skills + "goose_doc_guide.md": helpers.read("goose", "goose_doc_guide.md"),
                skills + "web_search.md": helpers.read("goose", "web_search.md"),
            },
            tree=[skills + "goose_doc_guide.md", skills + "web_search.md", skills + "sub/other.md", skills + "x.txt"],
        )
        # Act
        got = self.ex.extract(ctx)
        # Assert
        self.assertEqual(got["skill"]["names"], ["goose-doc-guide", "web-search"])
        self.assertIn("goal", got["command_agent"]["names"])

    def test_a_changed_format_is_an_error_not_an_empty_result(self):
        # Arrange / Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_agent_commands("fn other() {}")
        with self.assertRaises(ValueError):
            self.ex.parse_session_commands("fn other() {}")
        with self.assertRaises(ValueError):
            self.ex.parse_session_commands("fn handle_slash_command(input: &str) {\n    match input { _ => None }\n}\n")


if __name__ == "__main__":
    unittest.main()
