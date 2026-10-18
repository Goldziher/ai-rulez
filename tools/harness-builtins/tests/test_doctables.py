import unittest

from hb import doctables


class DocTablesTest(unittest.TestCase):
    def test_split_row_keeps_pipes_inside_code_and_escaped_pipes(self):
        # Arrange
        line = "| `/copy [n|btw]` | `/x [a\\|b]` | plain |"
        # Act
        got = doctables.split_row(line)
        # Assert
        self.assertEqual(got, ["`/copy [n|btw]`", "`/x [a|b]`", "plain"])

    def test_md_tables_records_the_nearest_heading(self):
        # Arrange
        text = "# A\n\n## Sessions\n\n| Command | Info |\n| - | - |\n| `/a` | one |\n\n| X |\n|---|\n| y |\n"
        # Act
        got = doctables.md_tables(text)
        # Assert
        self.assertEqual([t["heading"] for t in got], ["Sessions", "Sessions"])
        self.assertEqual(got[0]["rows"], [["`/a`", "one"]])

    def test_html_tables_turns_code_into_backticks_and_decodes_entities(self):
        # Arrange
        html = (
            "<h3>Slash &amp; more</h3><table><thead><tr><th>Command</th></tr></thead>"
            "<tbody><tr><td><code>/rename &lt;name&gt;</code>, <code>/help</code></td></tr></tbody></table>"
        )
        # Act
        got = doctables.html_tables(html)
        # Assert
        self.assertEqual(got[0]["heading"], "Slash & more")
        self.assertEqual(got[0]["rows"], [["`/rename <name>`, `/help`"]])
        self.assertEqual(doctables.slash_names(got[0]["rows"][0][0]), ["rename", "help"])

    def test_alias_helpers(self):
        # Arrange
        a = "Start a new chat. `/new` and `/newchat` are aliases."
        b = "Alias for `/branch`. Copies history."
        # Act / Assert
        self.assertEqual(doctables.aliases_in(a, "clear"), ["new", "newchat"])
        self.assertEqual(doctables.aliases_in(b, "fork"), [])
        self.assertEqual(doctables.alias_target(b), "branch")
        self.assertIsNone(doctables.alias_target(a))


if __name__ == "__main__":
    unittest.main()
