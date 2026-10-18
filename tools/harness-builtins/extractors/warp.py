"""Warp: slash commands of the app and of the Agent CLI (markdown pages; a page's .md form is read)."""

import re

from hb import doctables

APP = "https://docs.warp.dev/agents/capabilities/slash-commands.md"
CLI = "https://docs.warp.dev/agents/cli/reference.md"
ROW = re.compile(r"^\|\s*`/([\w:.-]+)[^`]*`\s*\|", re.MULTILINE)


def parse_commands(text: str, heading: str, header: str) -> list[str]:
    """First-column names of the table under heading. Cells of the app page span several lines, so the
    rows are read by their first column rather than split into cells."""
    found = doctables.find_tables(doctables.md_tables(text), heading, [header])
    if len(found) != 1:
        raise ValueError(f"expected one table under {heading!r}, found {len(found)}")
    at = re.search(r"^#{1,6}\s+" + re.escape(heading) + r"\s*$", text, re.MULTILINE)
    section = text[at.end() :]
    end = re.search(r"^#{1,6}\s", section, re.MULTILINE)
    names = ROW.findall(section[: end.start()] if end else section)
    if not names:
        raise ValueError(f"no commands under {heading!r}")
    return names


def extract(ctx) -> dict:
    app = parse_commands(ctx.get(APP), "Static slash commands", "Slash Command")
    # the CLI group lists what the app group does not: a name belongs to one group of a kind
    cli = [n for n in parse_commands(ctx.get(CLI), "Slash commands", "Command") if n not in app]
    return {"app": {"names": app}, "cli": {"names": cli}}
