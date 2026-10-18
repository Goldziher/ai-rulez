import unittest

from tests import helpers


class CodeBuddyTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("codebuddy")

    def test_parse_commands_reads_the_html_table(self):
        # Arrange
        text = helpers.read("codebuddy", "slash-commands.html")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertEqual(got["names"], ["help", "agents", "branch", "config", "model", "model:lite", "fork"])

    def test_parse_commands_makes_a_row_that_is_an_alias_of_another_an_alias(self):
        # Arrange
        text = helpers.read("codebuddy", "slash-commands.html")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertEqual(got["aliases"], {"branch": ["fork"]})

    def test_parse_commands_ignores_other_tables(self):
        # Arrange
        text = helpers.read("codebuddy", "slash-commands.html")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertNotIn("description", got["names"])

    def test_parse_commands_rejects_a_page_rendered_without_the_table(self):
        # Arrange
        text = "<html><body><div id='app'></div></body></html>"
        # Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_commands(text)

    def test_extract_reads_the_page(self):
        # Arrange
        ctx = helpers.context(files={self.ex.URL: helpers.read("codebuddy", "slash-commands.html")})
        # Act
        got = self.ex.extract(ctx)
        # Assert
        self.assertIn("model:lite", got["command"]["names"])


if __name__ == "__main__":
    unittest.main()
