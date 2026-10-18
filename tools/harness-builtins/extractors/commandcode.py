"""Command Code: the built-in command tables of the slash command reference (rendered HTML)."""

from hb import doctables

URL = "https://commandcode.ai/docs/reference/slash-commands"


def parse_commands(text: str) -> dict:
    """Every Command/Description table. Aliases are the other commands of the first cell
    ("`/clear` (`/new`)", "`/feedback [title]` (alias `/issue`)"); "Alias of `/x`" in a description
    makes the row an alias of x, and the row stays a command name."""
    tables = doctables.find_tables(doctables.html_tables(text), headers=["Command", "Description"])
    if not tables:
        raise ValueError("no Command/Description tables")
    out = doctables.Collector()
    for table in tables:
        for row in table["rows"]:
            names = doctables.slash_names(row[0])
            if not names:
                raise ValueError(f"row without a slash command: {row[0]!r}")
            out.add(names[0], [n for n in names[1:] if n != names[0]])
            owner = doctables.alias_target(row[1])
            if owner:
                out.add(owner, [names[0]])
    return out.result()


def extract(ctx) -> dict:
    return {"command": parse_commands(ctx.get(URL))}
