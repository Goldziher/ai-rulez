"""Devin CLI: interactive slash commands and the built-in subagent profiles."""

import re

from hb import doctables

COMMANDS_URL = "https://docs.devin.ai/cli/reference/commands.md"
SUBAGENTS_URL = "https://docs.devin.ai/cli/subagents.md"
HAS_ALIASES = re.compile(r"`/([A-Za-z0-9][\w-]*)` has aliases?((?:\s*(?:,|and)?\s*`/[A-Za-z0-9][\w-]*`)+)")


def parse_commands(text: str) -> dict:
    """The tables of the "Slash Commands" section. Aliases come from the description of a row
    ("Alias: `/new`", "(second name for `/x`)") or from a note ("`/bypass` has aliases `/yolo`")."""
    start = text.find("\n## Slash Commands")
    if start < 0:
        raise ValueError("no Slash Commands section")
    end = text.find("\n## ", start + 1)
    section = text[start : end if end > 0 else len(text)]
    tables = doctables.find_tables(doctables.md_tables(section), headers=["Command", "Description"])
    if len(tables) < 5:
        raise ValueError(f"expected the slash command tables, found {len(tables)}")
    out = doctables.Collector()
    for table in tables:
        for row in table["rows"]:
            names = doctables.slash_names(row[0])
            if len(names) != 1:
                raise ValueError(f"row without exactly one slash command: {row[0]!r}")
            name = names[0]
            out.add(name, doctables.aliases_in(row[1], name))
            owner = doctables.alias_target(row[1])
            if owner:
                out.add(owner, [name])
    for m in HAS_ALIASES.finditer(section):
        out.add(m.group(1), doctables.slash_names(m.group(2)))
    return out.result()


def parse_profiles(text: str) -> dict:
    """The names in the table of built-in subagent profiles."""
    tables = doctables.find_tables(doctables.md_tables(text), heading="Subagent Profiles", headers=["Profile"])
    if len(tables) != 1:
        raise ValueError(f"expected one Subagent Profiles table, found {len(tables)}")
    out = doctables.Collector()
    for row in tables[0]["rows"]:
        m = re.fullmatch(r"`([a-z_]+)`", row[0])
        if not m:
            raise ValueError(f"row without a profile name: {row[0]!r}")
        out.add(m.group(1))
    return out.result()


def extract(ctx) -> dict:
    return {
        "command": parse_commands(ctx.get(COMMANDS_URL)),
        "agent": parse_profiles(ctx.get(SUBAGENTS_URL)),
    }
