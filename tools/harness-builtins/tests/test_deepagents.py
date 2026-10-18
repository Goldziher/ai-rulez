import unittest

from tests import helpers


class DeepAgentsTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("deepagents")

    def test_parse_catalog_reads_public_hidden_and_aliases(self):
        # Arrange
        text = helpers.read("deepagents", "COMMANDS.md")
        # Act
        got = self.ex.parse_catalog(text)
        # Assert
        self.assertEqual(len(got["names"]), 48)
        self.assertIn("debug-error", got["names"])
        self.assertIn("force-clear", got["names"])
        self.assertEqual(got["aliases"]["offload"], ["compact"])
        self.assertEqual(got["aliases"]["quit"], ["q"])
        self.assertEqual(len(got["aliases"]), 6)

    def test_parse_catalog_rejects_a_count_that_does_not_match_the_heading(self):
        # Arrange
        text = helpers.read("deepagents", "COMMANDS.md").replace("## Public (46)", "## Public (47)")
        # Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_catalog(text)

    def test_parse_catalog_without_a_public_section_is_an_error(self):
        # Arrange / Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_catalog("# Slash command catalog\n\nnothing here\n")

    def test_parse_default_subagent(self):
        # Arrange
        text = helpers.read("deepagents", "subagents.py")
        # Act
        got = self.ex.parse_default_subagent(text)
        # Assert
        self.assertEqual(got, "general-purpose")
        with self.assertRaises(ValueError):
            self.ex.parse_default_subagent("X = 1\n")

    def test_extract_names_skills_by_front_matter_and_directory(self):
        # Arrange
        skills = self.ex.SKILLS
        ctx = helpers.context(
            "langchain-ai/deepagents",
            "r1",
            files={
                self.ex.CATALOG: helpers.read("deepagents", "COMMANDS.md"),
                self.ex.SUBAGENTS: helpers.read("deepagents", "subagents.py"),
                skills + "remember/SKILL.md": helpers.read("deepagents", "remember_SKILL.md"),
                skills + "skill-creator/SKILL.md": "no front matter\n",
            },
            tree=[
                skills + "remember/SKILL.md",
                skills + "skill-creator/SKILL.md",
                skills + "skill-creator/scripts/SKILL.md",
                skills + "__init__.py",
            ],
        )
        # Act
        got = self.ex.extract(ctx)
        # Assert
        self.assertEqual(got["skill"]["names"], ["remember", "skill-creator"])
        self.assertEqual(got["agent"]["names"], ["general-purpose"])
        self.assertIn("goal", got["command"]["names"])


if __name__ == "__main__":
    unittest.main()
