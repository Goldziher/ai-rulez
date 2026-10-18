import unittest

from tests import helpers


class ReasonixTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("reasonix")

    def test_parse_slash_reads_names_and_aliases_without_slashes_and_skips_comments(self):
        # Arrange
        text = helpers.read("reasonix", "slash_registry.go")
        # Act
        got = self.ex.parse_slash(text)
        # Assert
        self.assertEqual(
            got,
            {
                "compact": [],
                "plugins": ["plugin"],
                "preset": ["work-mode", "profile"],
                "docs": ["reasonix:docs"],
                "quit": ["exit"],
            },
        )

    def test_parse_slash_fails_when_the_registry_is_gone(self):
        # Arrange / Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_slash("package cli\n")

    def test_split_fallback_separates_the_command_with_a_qualified_alias(self):
        # Arrange
        specs = self.ex.parse_slash(helpers.read("reasonix", "slash_registry.go"))
        # Act
        fallback, rest = self.ex.split_fallback(specs)
        # Assert
        self.assertEqual(fallback, {"names": ["docs"], "aliases": {"docs": ["reasonix:docs"]}})
        self.assertEqual(rest["names"], ["compact", "plugins", "preset", "quit"])
        self.assertEqual(rest["aliases"]["quit"], ["exit"])

    def test_parse_builtin_skills_reads_entry_names_only(self):
        # Arrange
        text = helpers.read("reasonix", "builtins.go")
        # Act
        got = self.ex.parse_builtin_skills(text)
        # Assert
        self.assertEqual(got, ["init", "explore"])

    def test_extract_adds_the_embedded_skill_directories(self):
        # Arrange
        e = self.ex.EMBEDDED
        ctx = helpers.context(
            "esengine/DeepSeek-Reasonix",
            "v2",
            files={
                self.ex.SLASH: helpers.read("reasonix", "slash_registry.go"),
                self.ex.BUILTINS: helpers.read("reasonix", "builtins.go"),
                e + "reasonix-guide/SKILL.md": helpers.read("reasonix", "reasonix-guide-SKILL.md"),
            },
            tree=[e + "embed.go", e + "reasonix-guide/SKILL.md", e + "reasonix-guide/refs/SKILL.md"],
        )
        # Act
        got = self.ex.extract(ctx)
        # Assert
        self.assertEqual(got["skill"]["names"], ["init", "explore", "reasonix-guide"])
        self.assertEqual(got["fallback"]["names"], ["docs"])
        self.assertEqual(len(got["command"]["names"]), 4)


if __name__ == "__main__":
    unittest.main()
