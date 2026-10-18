"""Poolside pool CLI: slash commands from the CLI reference (markdown)."""

import re

from hb import cells, doctables

REFERENCE = "https://docs.poolside.ai/cli/cli-reference.md"
AS_ALIAS = re.compile(r"[Uu]se `/([\w:.-]+)` as an alias")


def parse_commands(text: str) -> dict:
    """Names and aliases of the tables under "Slash commands". "`/new` or `/clear`" lists an alias
    after "or"; a description can also say "Use `/x` as an alias"."""
    names, aliases, seen = [], {}, False
    for table in doctables.md_tables(text):
        if table["headers"][:1] != ["Command"] or not table["heading"].endswith("slash commands"):
            continue
        seen = True
        for row in table["rows"]:
            listed = cells.slash_names(row[0])
            if not listed:
                raise ValueError(f"cannot read a command from {row[0]!r}")
            primary = listed[0]
            al = listed[1:] + AS_ALIAS.findall(row[1] if len(row) > 1 else "")
            if primary not in names:
                names.append(primary)
            if al:
                aliases.setdefault(primary, [])
                aliases[primary] += [a for a in al if a not in aliases[primary]]
    if not seen:
        raise ValueError("no slash command table found")
    return {"names": names, "aliases": aliases}


def extract(ctx) -> dict:
    return {"command": parse_commands(ctx.get(REFERENCE))}
