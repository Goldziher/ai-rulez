"""JetBrains Junie CLI: slash commands from the slash commands page (HTML table)."""

from hb import cells, doctables

PAGE = "https://junie.jetbrains.com/docs/slash-commands.html"


def parse_commands(text: str) -> dict:
    """Names of the "Built-in slash commands" table; aliases come from "Alias: /x" sentences in the
    description."""
    found = doctables.find_tables(doctables.html_tables(text), "Built-in slash commands", ["Command"])
    if len(found) != 1:
        raise ValueError(f"expected one built-in slash command table, found {len(found)}")
    names, aliases = [], {}
    for row in found[0]["rows"]:
        listed = cells.slash_names(row[0])
        if len(listed) != 1:
            raise ValueError(f"cannot read a command from {row[0]!r}")
        names.append(listed[0])
        _, al = cells.split_alias_clause(row[1])
        if al:
            aliases[listed[0]] = al
    return {"names": names, "aliases": aliases}


def extract(ctx) -> dict:
    return {"command": parse_commands(ctx.get(PAGE))}
