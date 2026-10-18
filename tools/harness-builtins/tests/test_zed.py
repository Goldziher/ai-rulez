import unittest

from tests import helpers


class ZedTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("zed")

    def test_parse_commands_reads_public_constants_and_skips_comments(self):
        # Arrange
        text = helpers.read("zed", "agent.rs")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertEqual(got, ["compact"])

    def test_parse_skills_reads_the_embedded_entries(self):
        # Arrange
        text = helpers.read("zed", "agent_skills.rs")
        # Act
        got = self.ex.parse_skills(text)
        # Assert
        self.assertEqual(got, ["create-skill"])

    def test_parse_fails_when_the_declarations_are_gone(self):
        # Arrange / Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_commands('pub const OTHER: &str = "x";')
        with self.assertRaises(ValueError):
            self.ex.parse_skills("const OTHER: &[(&str, &str)] = &[];")

    def test_parse_skills_reads_several_entries(self):
        # Arrange
        text = 'const BUILTIN_SKILL_ENTRIES: &[(&str, &str)] = &[\n    ("a", A),\n    ("b", B),\n];'
        # Act / Assert
        self.assertEqual(self.ex.parse_skills(text), ["a", "b"])

    def test_extract_combines_both_sources(self):
        # Arrange
        ctx = helpers.context(
            "zed-industries/zed",
            "abc",
            files={
                self.ex.AGENT: helpers.read("zed", "agent.rs"),
                self.ex.SKILLS: helpers.read("zed", "agent_skills.rs"),
            },
        )
        # Act
        got = self.ex.extract(ctx)
        # Assert
        self.assertEqual(got, {"command": {"names": ["compact"]}, "skill": {"names": ["create-skill"]}})


if __name__ == "__main__":
    unittest.main()
