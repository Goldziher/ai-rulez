import unittest

from tests import helpers


class CodebuffTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("codebuff")
        self.slash = helpers.read("codebuff", "slash-commands.ts")

    def test_parse_commands_reads_literal_entries_and_skips_comments_and_spreads(self):
        # Arrange / Act
        got = self.ex.parse_commands(self.slash)
        # Assert
        self.assertEqual(got, {"help": ["h", "?"], "plan": [], "exit": ["quit", "q"]})

    def test_parse_commands_fails_when_the_list_is_gone(self):
        # Arrange / Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_commands("export const SLASH_COMMANDS = []\n")

    def test_parse_modes_derives_mode_commands_from_the_agent_modes(self):
        # Arrange
        constants = helpers.read("codebuff", "constants.ts")
        # Act
        got = self.ex.parse_modes(self.slash, constants)
        # Assert
        self.assertEqual(got["mode:lite"], ["model:lite"])
        self.assertEqual(list(got), ["mode:default", "mode:lite", "mode:max", "mode:plan"])

    def test_parse_modes_fails_when_the_template_changed(self):
        # Arrange
        changed = self.slash.replace("mode:", "m:")
        # Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_modes(changed, helpers.read("codebuff", "constants.ts"))

    def test_extract_combines_literal_and_mode_commands(self):
        # Arrange
        ctx = helpers.context(
            "CodebuffAI/codebuff",
            "abc",
            files={self.ex.SLASH: self.slash, self.ex.CONSTANTS: helpers.read("codebuff", "constants.ts")},
        )
        # Act
        got = self.ex.extract(ctx)
        # Assert
        self.assertEqual(
            got["command"]["names"], ["help", "plan", "exit", "mode:default", "mode:lite", "mode:max", "mode:plan"]
        )
        self.assertEqual(got["command"]["aliases"]["mode:max"], ["model:max"])


if __name__ == "__main__":
    unittest.main()
