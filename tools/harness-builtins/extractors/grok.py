"""Grok Build: slash commands and built-in subagent types from docs.x.ai (markdown pages)."""

from hb import cells, doctables

COMMANDS = "https://docs.x.ai/build/modes-and-commands.md"
AGENTS = "https://docs.x.ai/build/features/subagents.md"
COMMAND_TABLES = ("Core TUI commands", "Shell-provided commands")


def parse_commands(text: str) -> dict:
    """Names and aliases of the command tables. A cell may list several commands (`/login`, `/logout`)."""
    tables = doctables.md_tables(text)
    names, aliases = [], {}
    for heading in COMMAND_TABLES:
        found = doctables.find_tables(tables, heading, ["Command"])
        if len(found) != 1:
            raise ValueError(f"expected one command table under {heading!r}, found {len(found)}")
        for row in found[0]["rows"]:
            head, al = cells.split_alias_clause(row[0])
            listed = cells.slash_names(head)
            if not listed:
                raise ValueError(f"no command in row {row[0]!r}")
            names += listed
            if al:
                aliases[listed[0]] = al
    return {"names": names, "aliases": aliases}


def parse_agents(text: str) -> list[str]:
    found = doctables.find_tables(doctables.md_tables(text), "Built-in types", ["Type"])
    if len(found) != 1:
        raise ValueError("built-in subagent types table not found")
    out = [t for row in found[0]["rows"] for t in cells.CODE.findall(row[0])]
    if not out:
        raise ValueError("no built-in subagent types found")
    return out


def extract(ctx) -> dict:
    return {"command": parse_commands(ctx.get(COMMANDS)), "agent": {"names": parse_agents(ctx.get(AGENTS))}}
