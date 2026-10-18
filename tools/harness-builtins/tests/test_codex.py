import unittest

from tests import helpers


class CodexTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("codex")

    def test_parse_roles_skips_commented_out_entries(self):
        # Arrange
        text = helpers.read("codex", "role.rs")
        # Act
        got = self.ex.parse_roles(text)
        # Assert
        self.assertEqual(got, ["default", "explorer", "worker"])

    def test_extract_reads_commands_aliases_skills_and_roles(self):
        # Arrange
        skill = "---\nname: imagegen\ndescription: x\n---\nbody\n"
        sample = self.ex.SAMPLES
        ctx = helpers.context(
            "openai/codex",
            "t1",
            files={
                self.ex.SLASH: helpers.read("codex", "slash_command.rs"),
                self.ex.ROLES: helpers.read("codex", "role.rs"),
                sample + "imagegen/SKILL.md": skill,
                sample + "skill-creator/SKILL.md": "no front matter\n",
            },
            tree=[sample + "imagegen/SKILL.md", sample + "skill-creator/SKILL.md", sample + "imagegen/assets/SKILL.md"],
        )
        # Act
        got = self.ex.extract(ctx)
        # Assert
        cmds = got["command"]
        self.assertIn("setup-default-sandbox", cmds["names"])
        self.assertIn("approve", cmds["names"])
        self.assertNotIn("auto-review", cmds["names"])
        self.assertIn("debug-m-drop", cmds["names"])
        self.assertEqual(cmds["aliases"], {"pwd": ["cwd"], "pets": ["pet"], "stop": ["clean"]})
        self.assertEqual(got["skill"]["names"], ["imagegen", "skill-creator"])
        self.assertEqual(got["agent"]["names"], ["default", "explorer", "worker"])


if __name__ == "__main__":
    unittest.main()
