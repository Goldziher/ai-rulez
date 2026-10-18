"""ZCode: built-in slash commands from the commands page (HTML table)."""

from hb import cells, doctables

PAGE = "https://zcode.z.ai/en/docs/commands"


def parse_commands(text: str) -> list[str]:
    """Names of the table headed "Command | Purpose" (the other table describes custom commands)."""
    found = doctables.find_tables(doctables.html_tables(text), headers=["Command", "Purpose"])
    if len(found) != 1:
        raise ValueError(f"expected one Command/Purpose table, found {len(found)}")
    names = [n for row in found[0]["rows"] for n in cells.slash_names(row[0])]
    if not names:
        raise ValueError("no commands found")
    return names


def extract(ctx) -> dict:
    return {"command": {"names": parse_commands(ctx.get(PAGE))}}
