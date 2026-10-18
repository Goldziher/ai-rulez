import unittest

from tests import helpers


class ReplitTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("replit")

    def test_parse_skills_splits_the_category_rows(self):
        # Arrange
        text = helpers.read("replit", "skills-directory.md")
        # Act
        got = self.ex.parse_skills(text)
        # Assert
        self.assertEqual(got[:3], ["Ad Creative", "AI SDR", "SEO Auditor"])
        self.assertIn("Ship Review", got)

    def test_parse_skills_leaves_out_the_partner_table(self):
        # Arrange
        text = helpers.read("replit", "skills-directory.md")
        # Act
        got = self.ex.parse_skills(text)
        # Assert
        self.assertNotIn("Stripe", got)

    def test_parse_skills_fails_when_the_table_is_gone(self):
        # Arrange / Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_skills("## Replit skills\n\nBrowse the picker.\n")

    def test_extract_returns_the_skill_key(self):
        # Arrange
        ctx = helpers.context(files={self.ex.PAGE: helpers.read("replit", "skills-directory.md")})
        # Act
        got = self.ex.extract(ctx)
        # Assert
        self.assertEqual(list(got), ["skill"])
