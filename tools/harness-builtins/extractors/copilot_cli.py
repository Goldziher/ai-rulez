"""GitHub Copilot CLI: slash commands, built-in agents and built-in app skills, read from the markdown
sources of docs.github.com (github/docs) at the pinned commit."""

import re

from hb import doctables

REFERENCE = "content/copilot/reference/copilot-cli-reference/cli-command-reference.md"
SKILLS = "content/copilot/reference/github-copilot-app-reference/built-in-skills.md"


def section(text: str, heading: str) -> str:
    """The text from `heading` (a markdown heading line) to the next heading of the same or a higher level."""
    m = re.search(rf"(?m)^(#+) {re.escape(heading)}\s*$", text)
    if not m:
        raise ValueError(f"no section {heading!r}")
    rest = text[m.end() :]
    end = re.search(rf"(?m)^#{{1,{len(m.group(1))}}} ", rest)
    return rest[: end.start()] if end else rest


def parse_commands(text: str) -> dict:
    """The first command of a cell names the command, the others are aliases (`/ask`, `/btw`)."""
    tables = doctables.find_tables(
        doctables.md_tables(section(text, "Slash commands in the interactive interface")),
        headers=["Command", "Purpose"],
    )
    if len(tables) != 1:
        raise ValueError(f"expected one slash command table, found {len(tables)}")
    out = doctables.Collector()
    for row in tables[0]["rows"]:
        names = doctables.slash_names(row[0])
        if not names:
            raise ValueError(f"row without a slash command: {row[0]!r}")
        out.add(names[0], names[1:])
    return out.result()


def parse_names(text: str, heading: str, header: str) -> dict:
    """The backticked first cells of the table under `heading`."""
    tables = doctables.md_tables(section(text, heading) if heading else text)
    tables = doctables.find_tables(tables, headers=[header])
    if len(tables) != 1:
        raise ValueError(f"expected one {header} table, found {len(tables)}")
    out = doctables.Collector()
    for row in tables[0]["rows"]:
        m = re.fullmatch(r"`([A-Za-z0-9][\w.-]*)`", row[0])
        if not m:
            raise ValueError(f"row without a name: {row[0]!r}")
        out.add(m.group(1))
    return out.result()


def extract(ctx) -> dict:
    reference = ctx.raw(REFERENCE)
    return {
        "command": parse_commands(reference),
        "agent": parse_names(reference, "Built-in agents", "Agent"),
        "skill": parse_names(ctx.raw(SKILLS), "", "Skill ID"),
    }
