"""Augment (Auggie): interactive slash commands from the CLI reference table."""

from hb import doctables

URL = "https://docs.augmentcode.com/cli/interactive.md"


def parse_commands(text: str) -> dict:
    """Rows of the tables under "Slash Commands"; a row may name several commands ("`/exit`, `/quit`")."""
    start = text.find("### Slash Commands")
    if start < 0:
        raise ValueError("no Slash Commands section")
    end = text.find("#### Custom and skill slash commands", start)
    if end < 0:
        raise ValueError("no end of the slash command tables")
    tables = doctables.find_tables(doctables.md_tables(text[start:end]), headers=["Command", "Description"])
    if not tables:
        raise ValueError("no slash command table found")
    out = doctables.Collector()
    for table in tables:
        for row in table["rows"]:
            names = doctables.slash_names(row[0])
            if not names:
                raise ValueError(f"row without a slash command: {row[0]!r}")
            for name in names:
                out.add(name)
    return out.result()


def extract(ctx) -> dict:
    return {"command": parse_commands(ctx.get(URL))}
