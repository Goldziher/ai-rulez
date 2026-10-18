"""Kiro: CLI slash commands (one heading each) and the IDE's predefined steering commands (markdown)."""

import re

from hb import cells, doctables

CLI = "https://kiro.dev/docs/reference/slash-commands.md"
IDE = "https://kiro.dev/docs/ide/chat/slash-commands.md"
HEADING = re.compile(r"^###\s+`/([\w:.-]+)`\s*$", re.MULTILINE)
ALIASES = re.compile(r"^Aliases?:\s*(.*)$", re.MULTILINE)


def parse_cli(text: str) -> dict:
    """Names from the level-3 headings "### `/name`" of the "Available commands" section (a heading
    with a subcommand, such as `/mcp auth`, is not matched). The line "Aliases: /exit, /q" under a
    heading lists its aliases."""
    start = text.find("## Available commands")
    if start < 0:
        raise ValueError("'Available commands' section not found")
    section = text[start:]
    heads = list(HEADING.finditer(section))
    if not heads:
        raise ValueError("no command headings found")
    names, aliases = [], {}
    for i, m in enumerate(heads):
        body = section[m.end() : heads[i + 1].start() if i + 1 < len(heads) else len(section)]
        body = re.split(r"^#{2,3} (?!`/)", body, flags=re.MULTILINE)[0]
        names.append(m.group(1))
        a = ALIASES.search(body)
        if a:
            aliases[m.group(1)] = [x.rstrip(".") for x in re.findall(r"/([\w:.-]+)", a.group(1))]
    return {"names": names, "aliases": aliases}


def parse_ide(text: str) -> list[str]:
    """The table under "Predefined by Kiro" (steering commands that ship with the IDE)."""
    found = [
        t for t in doctables.md_tables(text) if t["headers"][:1] == ["Command"] and t["heading"] == "Steering files"
    ]
    if len(found) != 1:
        raise ValueError(f"expected one predefined steering command table, found {len(found)}")
    names = [n for row in found[0]["rows"] for n in cells.slash_names(row[0])]
    if not names:
        raise ValueError("no predefined steering commands found")
    return names


def extract(ctx) -> dict:
    return {"cli": parse_cli(ctx.get(CLI)), "ide": {"names": parse_ide(ctx.get(IDE))}}
