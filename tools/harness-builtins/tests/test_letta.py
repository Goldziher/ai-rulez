import unittest

from tests import helpers


class LettaTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("letta")

    def test_parse_commands_reads_keys_including_hidden_and_skips_comments(self):
        # Arrange
        text = helpers.read("letta", "registry.ts")
        # Act
        got = self.ex.parse_commands(text)
        # Assert
        self.assertEqual(got, ["agents", "clear-messages", "chdir", "cd", "pinned"])

    def test_parse_commands_fails_when_the_registry_is_gone(self):
        # Arrange / Act / Assert
        with self.assertRaises(ValueError):
            self.ex.parse_commands("export const other = {};\n")

    def test_extract_names_skills_by_id_then_name_then_directory(self):
        # Arrange: context-doctor sets `id`, browser-use only `name`, plain has no front matter
        sk = self.ex.SKILLS
        ag = self.ex.SUBAGENTS
        ctx = helpers.context(
            "letta-ai/letta-code",
            "v1",
            files={
                self.ex.REGISTRY: helpers.read("letta", "registry.ts"),
                sk + "context-doctor/SKILL.md": helpers.read("letta", "context-doctor-SKILL.md"),
                sk + "browser-use/SKILL.md": "---\nname: browser-use\n---\n",
                sk + "plain/SKILL.md": "no front matter\n",
                ag + "memory.md": "---\nname: memory\n---\n",
                ag + "memory-v2.md": "---\nname: memory\n---\n",
                ag + "fork.md": "---\ndescription: x\n---\n",
            },
            tree=[
                sk + "context-doctor/SKILL.md",
                sk + "browser-use/SKILL.md",
                sk + "plain/SKILL.md",
                sk + "plain/notes.txt",
                sk + "plain/references/api.md",
                sk + "plain/references/SKILL.md",
                ag + "sub/deep.md",
                ag + "memory.md",
                ag + "memory-v2.md",
                ag + "fork.md",
            ],
        )
        # Act
        got = self.ex.extract(ctx)
        # Assert
        self.assertEqual(got["skill"]["names"], ["browser-use", "context-doctor", "plain"])
        self.assertEqual(got["agent"]["names"], ["fork", "memory"])
        self.assertEqual(len(got["command"]["names"]), 5)

    def test_extract_fails_when_there_are_no_skills(self):
        # Arrange
        ctx = helpers.context(
            "letta-ai/letta-code", "v1", files={self.ex.REGISTRY: helpers.read("letta", "registry.ts")}, tree=[]
        )
        # Act / Assert
        with self.assertRaises(ValueError):
            self.ex.extract(ctx)


if __name__ == "__main__":
    unittest.main()
