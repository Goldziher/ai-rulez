"""GitHub Copilot in VS Code: the slash command table of the chat reference (rendered HTML)."""

from hb import doctables

URL = "https://code.visualstudio.com/docs/copilot/reference/copilot-vscode-features"


def parse_commands(text: str) -> dict:
    """Each cell lists several commands, none of them aliases of another."""
    tables = doctables.find_tables(doctables.html_tables(text), heading="Slash commands", headers=["Command"])
    if len(tables) != 1:
        raise ValueError(f"expected one Slash commands table, found {len(tables)}")
    out = doctables.Collector()
    for row in tables[0]["rows"]:
        names = doctables.slash_names(row[0])
        if row[0].startswith("`/<"):  # the placeholder row for a custom prompt file, not a command
            continue
        if not names:
            raise ValueError(f"row without a slash command: {row[0]!r}")
        for name in names:
            out.add(name)
    return out.result()


def extract(ctx) -> dict:
    return {"command": parse_commands(ctx.get(URL))}
