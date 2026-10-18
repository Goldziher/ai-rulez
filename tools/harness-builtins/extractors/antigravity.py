"""Antigravity CLI: the "Core slash commands" table of the CLI commands reference."""

from hb import doctables

URL = "https://antigravity.google/docs/cli/reference.md"


def parse_commands(text: str) -> dict:
    tables = doctables.find_tables(
        doctables.md_tables(text), heading="Core slash commands", headers=["Command", "Category", "Alias"]
    )
    if len(tables) != 1:
        raise ValueError(f"expected one Core slash commands table, found {len(tables)}")
    out = doctables.Collector()
    for row in tables[0]["rows"]:
        names = doctables.slash_names(row[0])
        if len(names) != 1:
            raise ValueError(f"row without exactly one slash command: {row[0]!r}")
        out.add(names[0], doctables.slash_names(row[2]))
    return out.result()


def extract(ctx) -> dict:
    return {"command": parse_commands(ctx.get(URL))}
