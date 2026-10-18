"""Cursor: CLI slash commands and bundled skills from the documentation tables."""

from hb import doctables

SLASH_URL = "https://cursor.com/docs/cli/reference/slash-commands.md"
SKILLS_URL = "https://cursor.com/docs/skills.md"


def parse_commands(text: str) -> dict:
    """The rows of the "Slash commands" table; a description saying "`/x` is an alias" names aliases."""
    tables = doctables.find_tables(doctables.md_tables(text), headers=["Command", "Description"])
    if len(tables) != 1:
        raise ValueError(f"expected one Command/Description table, found {len(tables)}")
    out = doctables.Collector()
    for row in tables[0]["rows"]:
        names = doctables.slash_names(row[0])
        if len(names) != 1:
            raise ValueError(f"row without exactly one slash command: {row[0]!r}")
        out.add(names[0], doctables.aliases_in(row[1], names[0]))
    return out.result()


def parse_skills(text: str) -> dict:
    """The names in the "Built-in Cursor skills" table."""
    tables = doctables.find_tables(doctables.md_tables(text), heading="Built-in Cursor skills", headers=["Skill"])
    if len(tables) != 1:
        raise ValueError(f"expected one built-in skills table, found {len(tables)}")
    out = doctables.Collector()
    for row in tables[0]["rows"]:
        names = doctables.slash_names(row[0])
        if len(names) != 1:
            raise ValueError(f"row without exactly one skill: {row[0]!r}")
        out.add(names[0])
    return out.result()


def extract(ctx) -> dict:
    return {"command": parse_commands(ctx.get(SLASH_URL)), "skill": parse_skills(ctx.get(SKILLS_URL))}
