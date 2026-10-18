"""Factory Droid CLI: slash commands from the CLI reference (markdown)."""

from hb import cells, doctables

REFERENCE = "https://docs.factory.com/droid-cli/cli-reference.md"


def parse_commands(text: str) -> dict:
    """Names and aliases of the "Slash commands" table. "(alias for `/cwd`)" marks /cd as a command of
    its own, so it is a name; "(alias: `exit`)" is an alias of the command."""
    found = doctables.find_tables(doctables.md_tables(text), "Slash commands", ["Command"])
    if len(found) != 1:
        raise ValueError(f"expected one slash command table, found {len(found)}")
    names, aliases = [], {}
    for row in found[0]["rows"]:
        head, al = cells.split_alias_clause(row[0] + " " + (row[1] if len(row) > 1 else ""))
        listed = cells.slash_names(row[0])
        if len(listed) != 1:
            raise ValueError(f"cannot read a command from {row[0]!r}")
        names.append(listed[0])
        if al:
            aliases[listed[0]] = al
    return {"names": names, "aliases": aliases}


def extract(ctx) -> dict:
    return {"command": parse_commands(ctx.get(REFERENCE))}
