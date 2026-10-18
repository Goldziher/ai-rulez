import unittest

from tests import helpers


class CortexTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("cortex")

    def test_parse_commands_takes_the_first_command_of_a_cell_and_the_rest_as_aliases(self):
        # Arrange
        text = helpers.read("cortex", "cli-reference.md")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertEqual(got["aliases"]["resume"], ["r", "sessions"])
        self.assertEqual(got["aliases"]["bashes"], ["shells", "tasks"])
        self.assertEqual(got["aliases"]["diff"], ["changes", "review"])
        self.assertNotIn("cls", got["names"])

    def test_parse_commands_counts_a_command_once_however_many_rows_use_it(self):
        # Arrange
        text = helpers.read("cortex", "cli-reference.md")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertEqual(got["names"].count("sql"), 1)
        self.assertEqual(got["names"].count("skill"), 1)
        self.assertIn("sh", got["names"])  # "`/sh`, `! <command>`"
        self.assertEqual(len(got["names"]), 36)

    def test_parse_skills_reads_skill_headings_only(self):
        # Arrange
        text = helpers.read("cortex", "bundled-skills.md")
        # Act
        got = self.ex.parse_skills(text)
        # Assert
        self.assertEqual(
            got["names"],
            ["cortex-code-guide", "warehouse", "snowflake-interactive", "setting-up-astro-project", "init", "airflow"],
        )

    def test_parse_skills_rejects_a_page_without_skill_headings(self):
        # Arrange
        text = "# CoCo CLI bundled skills\n\nThe catalog moved.\n"
        # Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_skills(text)

    def test_parse_commands_rejects_a_changed_layout(self):
        # Arrange
        text = "### Slash commands\n\nno tables\n### Session storage\n"
        # Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_commands(text)

    def test_extract_reads_both_pages(self):
        # Arrange
        ctx = helpers.context(
            files={
                self.ex.CLI_URL: helpers.read("cortex", "cli-reference.md"),
                self.ex.SKILLS_URL: helpers.read("cortex", "bundled-skills.md"),
            }
        )
        # Act
        got = self.ex.extract(ctx)
        # Assert
        self.assertEqual(set(got), {"command", "skill"})


if __name__ == "__main__":
    unittest.main()
