import unittest

from tests import helpers


class CodewhaleTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("codewhale")

    def test_parse_commands_reads_info_literals_ignoring_comments(self):
        # Arrange
        text = helpers.read("codewhale", "config_mod.rs")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertEqual(got["workbar"], ["rail", "sidebar"])
        self.assertEqual(got["mode"], ["jihua", "zidong"])
        self.assertEqual(got["login"], [])

    def test_parse_commands_reads_registration_macro_calls(self):
        # Arrange
        text = helpers.read("codewhale", "debug_undo.rs")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertEqual(got, {"edit": [], "diff": [], "undo": [], "retry": ["chongshi"]})

    def test_parse_commands_skips_commented_out_entries(self):
        # Arrange
        text = '// name: "gone", aliases: &["x"],\nname: "kept", aliases: &[],\n'
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertEqual(got, {"kept": []})

    def test_split_commands_separates_protected_builtins(self):
        # Arrange
        protected = self.ex.parse_protected(helpers.read("codewhale", "user_registry.rs"))
        commands = {"trust": ["xinren"], "help": ["?"], "plugin": []}
        # Act
        prot, rest = self.ex.split_commands(commands, protected)
        # Assert
        self.assertEqual(prot, {"names": ["trust", "plugin"], "aliases": {"trust": ["xinren"]}})
        self.assertEqual(rest, {"names": ["help"], "aliases": {"help": ["?"]}})

    def test_parse_protected_fails_when_the_list_is_gone(self):
        # Arrange / Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_protected("const OTHER: &[&str] = &[];")

    def test_parse_skills_reads_the_matrix(self):
        # Arrange
        text = helpers.read("codewhale", "skills-catalog-matrix.json")
        # Act
        got = self.ex.parse_skills(text)
        # Assert
        self.assertEqual(got, ["skill-creator", "delegate", "docx", "documents"])

    def test_parse_roles_reads_roles_and_aliases(self):
        # Arrange
        text = helpers.read("codewhale", "SUBAGENTS.md")
        # Act
        got = self.ex.parse_roles(text)
        # Assert
        self.assertEqual(got["names"][:3], ["general", "explore", "planner"])
        self.assertEqual(len(got["names"]), 8)
        self.assertEqual(got["aliases"]["advisor"], ["consultant", "oracle"])
        self.assertNotIn("custom", got["aliases"])

    def test_parse_roles_fails_when_the_table_is_gone(self):
        # Arrange / Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_roles("no table here\n")

    def test_extract_combines_the_sources(self):
        # Arrange
        g = self.ex.GROUPS
        ctx = helpers.context(
            "codewhale-hq/Codewhale",
            "v1",
            files={
                g + "config/mod.rs": helpers.read("codewhale", "config_mod.rs"),
                g + "config/mod_tests.rs": 'name: "bogus", aliases: &[],',
                self.ex.USER_REGISTRY: helpers.read("codewhale", "user_registry.rs"),
                self.ex.MATRIX: helpers.read("codewhale", "skills-catalog-matrix.json"),
                self.ex.SUBAGENTS: helpers.read("codewhale", "SUBAGENTS.md"),
            },
            tree=[g + "config/mod.rs", g + "config/mod_tests.rs"],
        )
        # Act
        got = self.ex.extract(ctx)
        # Assert
        self.assertEqual(got["command_protected"]["names"], ["login", "auth", "settings", "mode"])
        self.assertEqual(got["command"]["names"], ["workbar", "pet", "statusline"])
        self.assertEqual(len(got["skill"]["names"]), 4)
        self.assertEqual(len(got["agent"]["names"]), 8)


if __name__ == "__main__":
    unittest.main()
