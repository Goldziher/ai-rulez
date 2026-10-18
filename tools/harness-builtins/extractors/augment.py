"""Augment (Auggie): interactive slash commands from the CLI reference tables."""

from hb import doctables

URL = "https://docs.augmentcode.com/cli/interactive.md"


def parse_commands(text: str) -> dict:
    """Rows of the tables under "Slash Commands" (common and additional commands)."""
    start = text.find("### Slash Commands")
    if start < 0:
        raise ValueError("no Slash Commands section")
    end = text.find("#### Custom and skill slash commands", start)
    if end < 0:
        raise ValueError("no end of the slash command tables")
    tables = doctables.find_tables(doctables.md_tables(text[start:end]), headers=["Command", "Description"])
    if len(tables) != 2:
        raise ValueError(f"expected two slash command tables, found {len(tables)}")
    out = doctables.Collector()
    for table in tables:
        for row in table["rows"]:
            names = doctables.slash_names(row[0])
            if len(names) != 1:
                raise ValueError(f"row without exactly one slash command: {row[0]!r}")
            out.add(names[0])
    return out.result()


def extract(ctx) -> dict:
    return {"command": parse_commands(ctx.get(URL))}
