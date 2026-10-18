"""CodeBuddy Code: the "Built-in Slash Commands" table of the CLI documentation (rendered HTML)."""

from hb import doctables

URL = "https://www.codebuddy.ai/docs/cli/slash-commands"


def parse_commands(text: str) -> dict:
    """A row whose description says "Alias for `/x`" is an alias of x and is still a command name."""
    tables = doctables.find_tables(
        doctables.html_tables(text), heading="Built-in Slash Commands", headers=["Command", "Parameters"]
    )
    if len(tables) != 1:
        raise ValueError(f"expected one Built-in Slash Commands table, found {len(tables)}")
    out = doctables.Collector()
    for row in tables[0]["rows"]:
        names = doctables.slash_names(row[0])
        if len(names) != 1:
            raise ValueError(f"row without exactly one slash command: {row[0]!r}")
        out.add(names[0])
        owner = doctables.alias_target(row[-1])
        if owner:
            out.add(owner, [names[0]])
    return out.result()


def extract(ctx) -> dict:
    return {"command": parse_commands(ctx.get(URL))}
