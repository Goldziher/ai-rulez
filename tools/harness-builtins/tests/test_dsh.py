import unittest

from tests import helpers


class DshTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("dsh")
        self.office = [
            "office",
            "office-docx",
            "office-pptx",
            "office-xlsx",
            "office-render",
            "office-recalculate",
            "office-export-pdf",
        ]

    def context(self, tree):
        return helpers.context(
            "deepseek-ai/deepseek-harness",
            "abc",
            files={
                self.ex.OFFICE_SRC: helpers.read("dsh", "skill-office-index.ts"),
                self.ex.ACL_SRC: helpers.read("dsh", "acl-skill.ts"),
            },
            tree=tree,
        )

    def test_parse_office_reads_the_skill_names_tuple(self):
        # Arrange
        text = helpers.read("dsh", "skill-office-index.ts")
        # Act
        got = self.ex.parse_office(text)
        # Assert
        self.assertEqual(got, self.office)

    def test_parse_acl_reads_the_constant(self):
        # Arrange / Act / Assert
        self.assertEqual(self.ex.parse_acl(helpers.read("dsh", "acl-skill.ts")), "diagnose-windows-sandbox-acl")

    def test_parse_fails_when_the_declarations_are_gone(self):
        # Arrange / Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_office("const OTHER = [] as const")
        with self.assertRaises(ValueError):
            self.ex.parse_acl("export const OTHER = 'x'")

    def test_extract_lists_the_office_and_acl_skills_separately(self):
        # Arrange
        tree = [f"{self.ex.OFFICE}assets/{n}/SKILL.md" for n in self.office]
        tree.append(f"{self.ex.ACL}assets/diagnose-windows-sandbox-acl/SKILL.md")
        # Act
        got = self.ex.extract(self.context(tree))
        # Assert
        self.assertEqual(got["office"]["names"], self.office)
        self.assertEqual(got["acl"]["names"], ["diagnose-windows-sandbox-acl"])

    def test_extract_fails_when_a_declared_skill_has_no_asset(self):
        # Arrange
        tree = [f"{self.ex.OFFICE}assets/{n}/SKILL.md" for n in self.office]
        # Act / Assert
        with self.assertRaises(ValueError):
            self.ex.extract(self.context(tree))


if __name__ == "__main__":
    unittest.main()
