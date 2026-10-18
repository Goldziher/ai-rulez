import unittest

from tests import helpers


class CursorTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("cursor")

    def test_parse_commands_reads_names_and_aliases_from_descriptions(self):
        # Arrange
        text = helpers.read("cursor", "slash-commands.md")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertIn("model", got["names"])
        self.assertIn("setup-terminal", got["names"])
        self.assertNotIn("auto-run", got["names"])
        self.assertEqual(got["aliases"]["clear"], ["new", "new-chat", "newchat"])
        self.assertEqual(got["aliases"]["shell"], ["sh", "run"])
        self.assertNotIn("setup-terminal", got["aliases"])

    def test_parse_skills_reads_the_builtin_table(self):
        # Arrange
        text = helpers.read("cursor", "skills.md")
        # Act
        got = self.ex.parse_skills(text)
        # Assert
        self.assertEqual(got["names"][:3], ["automate", "autopilot", "canvas"])
        self.assertIn("update-cursor-settings", got["names"])
        self.assertEqual(len(got["names"]), 19)

    def test_parse_commands_rejects_a_page_without_the_table(self):
        # Arrange
        text = "# Slash commands\n\nThe table moved.\n"
        # Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_commands(text)

    def test_parse_skills_rejects_a_row_without_a_skill(self):
        # Arrange
        text = "## Built-in Cursor skills\n\n| Skill | What |\n| - | - |\n| plain | x |\n"
        # Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_skills(text)

    def test_extract_reads_both_pages(self):
        # Arrange
        ctx = helpers.context(
            files={
                self.ex.SLASH_URL: helpers.read("cursor", "slash-commands.md"),
                self.ex.SKILLS_URL: helpers.read("cursor", "skills.md"),
            }
        )
        # Act
        got = self.ex.extract(ctx)
        # Assert
        self.assertEqual(set(got), {"command", "skill"})
        self.assertIn("review", got["skill"]["names"])


if __name__ == "__main__":
    unittest.main()
