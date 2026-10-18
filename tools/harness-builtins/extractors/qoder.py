"""Qoder CLI: slash commands, bundled skills and built-in subagents from docs.qoder.com (markdown)."""

from hb import cells, doctables

SLASH = "https://docs.qoder.com/cli/slash-reference.md"
BUILTINS = "https://docs.qoder.com/cli/builtins-reference.md"
SKILL_COMMANDS = "Built-in Skills"


def parse_commands(text: str) -> dict:
    """Names and aliases of every command table except "Built-in Skills" (those are the skills)."""
    names, aliases = [], {}
    for table in doctables.md_tables(text):
        if table["headers"][:1] != ["Command"] or SKILL_COMMANDS.casefold() in table["heading"].casefold():
            continue
        for row in table["rows"]:
            listed = cells.slash_names(row[0])
            if len(listed) != 1:
                raise ValueError(f"cannot read a command from {row[0]!r}")
            names.append(listed[0])
            _, al = cells.split_alias_clause(row[1])
            if al:
                aliases[listed[0]] = al
    if not names:
        raise ValueError("no slash command table found")
    return {"names": names, "aliases": aliases}


def parse_builtins(text: str, first_header: str) -> list[str]:
    """Names (first backticked token of column one) of the table whose first header is first_header."""
    found = [t for t in doctables.md_tables(text) if t["headers"][:1] == [first_header]]
    if len(found) != 1:
        raise ValueError(f"expected one table headed {first_header!r}, found {len(found)}")
    out = [cells.CODE.findall(row[0])[0] for row in found[0]["rows"] if cells.CODE.findall(row[0])]
    if not out:
        raise ValueError(f"table headed {first_header!r} has no names")
    return out


def extract(ctx) -> dict:
    page = ctx.get(BUILTINS)
    return {
        "command": parse_commands(ctx.get(SLASH)),
        "skill": {"names": parse_builtins(page, "Skill")},
        "agent": {"names": parse_builtins(page, "Agent")},
    }
