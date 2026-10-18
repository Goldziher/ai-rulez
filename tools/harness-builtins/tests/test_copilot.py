import unittest

from tests import helpers


class CopilotTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("copilot")

    def test_parse_commands_lists_every_command_of_a_cell_as_a_name(self):
        # Arrange
        text = helpers.read("copilot", "copilot-vscode-features.html")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertEqual(
            got["names"],
            ["plan", "explain", "fix", "tests", "setupTests", "rename", "help"]
            + ["agents", "instructions", "skills", "prompts", "hooks", "sandbox"],
        )
        self.assertEqual(got["aliases"], {})

    def test_parse_commands_keeps_the_case_of_a_name(self):
        # Arrange
        text = helpers.read("copilot", "copilot-vscode-features.html")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertIn("setupTests", got["names"])

    def test_parse_commands_skips_the_placeholder_row_for_prompt_files(self):
        # Arrange
        text = helpers.read("copilot", "copilot-vscode-features.html")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertNotIn("<name>", got["names"])

    def test_parse_commands_rejects_a_page_without_the_table(self):
        # Arrange
        text = "<h3>Slash commands</h3><p>Type /</p>"
        # Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_commands(text)

    def test_extract_reads_the_page(self):
        # Arrange
        ctx = helpers.context(files={self.ex.URL: helpers.read("copilot", "copilot-vscode-features.html")})
        # Act
        got = self.ex.extract(ctx)
        # Assert
        self.assertIn("plan", got["command"]["names"])


if __name__ == "__main__":
    unittest.main()
