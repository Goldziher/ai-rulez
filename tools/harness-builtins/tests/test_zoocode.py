import unittest

from tests import helpers


class ZoocodeTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("zoocode")

    def test_parse_commands_reads_the_record_keys_not_nested_fields(self):
        # Arrange
        text = helpers.read("zoocode", "built-in-commands.ts")
        # Act / Assert
        self.assertEqual(self.ex.parse_commands(text), ["init"])

    def test_parse_commands_reads_several_and_quoted_keys(self):
        # Arrange
        text = 'const BUILT_IN_COMMANDS: Record<string, X> = {\n\tinit: {\n\t\tname: "init",\n\t},\n\t"deep-dive": {\n\t},\n}\n'
        # Act / Assert
        self.assertEqual(self.ex.parse_commands(text), ["init", "deep-dive"])

    def test_parse_commands_fails_when_the_record_is_gone_or_empty(self):
        for text in ("export const x = {}\n", "const BUILT_IN_COMMANDS: Record<string, X> = {\n}\n"):
            with self.subTest(text), self.assertRaises(ValueError):
                self.ex.parse_commands(text)

    def test_parse_modes_reads_the_slug_of_each_default_mode(self):
        # Arrange
        text = helpers.read("zoocode", "mode.ts")
        # Act / Assert
        self.assertEqual(self.ex.parse_modes(text), ["architect", "code", "ask", "orchestrator"])

    def test_parse_modes_ignores_slugs_of_nested_or_commented_out_fields(self):
        text = 'export const DEFAULT_MODES = [\n\t{\n\t\tslug: "a",\n\t\tgroups: [{\n\t\t\tslug: "nested",\n\t\t}],\n\t},\n] as const\n'
        self.assertEqual(self.ex.parse_modes(text), ["a"])

    def test_parse_modes_fails_when_the_array_moved(self):
        with self.assertRaises(ValueError):
            self.ex.parse_modes("export const OTHER = []\n")

    def test_extract_returns_a_result_for_each_group(self):
        # Arrange
        ctx = helpers.context(
            "Zoo-Code-Org/Zoo-Code",
            "v3.90.0",
            files={
                self.ex.COMMANDS: helpers.read("zoocode", "built-in-commands.ts"),
                self.ex.MODES: helpers.read("zoocode", "mode.ts"),
            },
        )
        # Act
        got = self.ex.extract(ctx)
        # Assert
        self.assertEqual(sorted(got), ["agent", "command"])
        self.assertEqual(got["command"]["names"], ["init"])


if __name__ == "__main__":
    unittest.main()
