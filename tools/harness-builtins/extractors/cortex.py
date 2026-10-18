"""Cortex Code (CoCo CLI): slash commands and bundled skills from the documentation."""

import re

from hb import doctables

CLI_URL = "https://docs.snowflake.com/en/user-guide/cortex-code/cli-reference.md"
SKILLS_URL = "https://docs.snowflake.com/en/user-guide/cortex-code/bundled-skills.md"


def parse_commands(text: str) -> dict:
    """The first slash command of a cell names the command, the others are its aliases. Rows that
    repeat a command with an argument (`/sql <query> --limit <n>`) add nothing."""
    start = text.find("\n### Slash commands")
    end = text.find("\n### ", start + 1)
    if start < 0 or end < 0:
        raise ValueError("no Slash commands section")
    tables = doctables.find_tables(doctables.md_tables(text[start:end]), headers=["Command"])
    if len(tables) < 5:
        raise ValueError(f"expected the slash command tables, found {len(tables)}")
    out = doctables.Collector()
    for table in tables:
        for row in table["rows"]:
            names = doctables.slash_names(row[0])
            if not names:
                raise ValueError(f"row without a slash command: {row[0]!r}")
            out.add(names[0], names[1:])
    return out.result()


def parse_skills(text: str) -> dict:
    """Every heading that is only a skill name in backticks."""
    out = doctables.Collector()
    for m in re.finditer(r"(?m)^#{3,4} `([a-z0-9][a-z0-9-]*)`\s*$", text):
        out.add(m.group(1))
    return out.result()


def extract(ctx) -> dict:
    return {"command": parse_commands(ctx.get(CLI_URL)), "skill": parse_skills(ctx.get(SKILLS_URL))}
