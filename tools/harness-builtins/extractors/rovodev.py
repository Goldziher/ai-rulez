"""Rovo Dev CLI: interactive slash commands from the CLI commands page (Atlassian support, rendered HTML).

The page lists each command as a paragraph that starts with an inline-code span, "<code>/new</code> -
Create a new session". A command with arguments or subcommands (/sessions new [title], /memory user)
is listed once per form; the name is the first word. The page carries the same content a second and
third time as escaped JSON, where the slash is written \\/, so only the rendered copy matches.
"""

import re

PAGE = "https://support.atlassian.com/rovo/docs/rovo-dev-cli-commands/"
ITEM = re.compile(r"<p[^>]*>\s*<code[^>]*>/([A-Za-z0-9][\w:.-]*)[^<]*</code>\s*-\s(.*?)</p>", re.DOTALL)
ALSO = re.compile(r"\(also\b(.*?)\)", re.DOTALL)
SLASH_CODE = re.compile(r"<code[^>]*>/([A-Za-z0-9][\w:.-]*)</code>")
START = "Interactive mode</h2>"


def parse_commands(text: str) -> dict:
    """First words of the /command paragraphs after the "Interactive mode" heading, once each, and the
    /x forms a description lists as "(also /quit, /q, exit)"."""
    start = text.find(START)
    if start < 0:
        raise ValueError(f"{START!r} not found")
    names, aliases = [], {}
    for m in ITEM.finditer(text, start):
        if m.group(1) not in names:
            names.append(m.group(1))
        also = ALSO.search(m.group(2))
        if also:
            aliases[m.group(1)] = SLASH_CODE.findall(also.group(1))
    if not names:
        raise ValueError("no slash commands found")
    return {"names": names, "aliases": {k: v for k, v in aliases.items() if v}}


def extract(ctx) -> dict:
    return {"command": parse_commands(ctx.get(PAGE))}
