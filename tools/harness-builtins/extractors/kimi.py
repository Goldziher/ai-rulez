"""Kimi Code CLI: slash commands, bundled skill commands and built-in sub-agents from the documentation."""

import re

COMMANDS = "https://moonshotai.github.io/kimi-code/en/reference/slash-commands.md"
AGENTS = "https://moonshotai.github.io/kimi-code/en/customization/agents.md"


def _cells(line: str) -> list[str]:
    # a pipe inside a code span is escaped as \| in the page
    parts = re.split(r"(?<!\\)\|", line.strip())
    return [c.strip() for c in parts[1:-1]]


def _ticked(cell: str) -> list[str]:
    return [m.removeprefix("/") for m in re.findall(r"`(/[^`\s]+)`", cell)]


def _tables(text: str) -> list[tuple[list[str], list[list[str]]]]:
    """[(header cells, [row cells])] of the markdown tables of a page."""
    out, cur = [], None
    for line in text.splitlines():
        if line.startswith("|"):
            cells = _cells(line)
            if cur is None:
                cur = (cells, [])
            elif not all(re.fullmatch(r":?-+:?|—", c) for c in cells):
                cur[1].append(cells)
        elif cur is not None:
            out.append(cur)
            cur = None
    if cur is not None:
        out.append(cur)
    return out


def _command_name(cell: str) -> str:
    m = re.match(r"`/([a-z][a-z0-9.-]*)", cell)
    if not m:
        raise ValueError(f"cannot read a command name from {cell!r}")
    return m.group(1)


def parse_commands(text: str) -> dict:
    """The tables headed `Command | Alias | ...`: the first word of the command cell and the aliases."""
    names, aliases = [], {}
    for header, rows in _tables(text):
        if header[:2] != ["Command", "Alias"]:
            continue
        for row in rows:
            name = _command_name(row[0])
            if name not in names:
                names.append(name)
            found = [a for a in _ticked(row[1]) if a != name]
            if found:
                aliases[name] = sorted(set(aliases.get(name, []) + found))
    if not names:
        raise ValueError("no 'Command | Alias' table found")
    return {"names": names, "aliases": aliases}


def parse_skill_commands(text: str) -> list[str]:
    """The built-in skill commands and the dotted sub-skill commands their descriptions list."""
    m = re.search(r"(?ms)^## Built-in skill commands\s*$(.*?)(?=^## )", text)
    if not m:
        raise ValueError("section 'Built-in skill commands' not found")
    names = []
    for header, rows in _tables(m.group(1)):
        if header[:2] != ["Command", "Description"]:
            continue
        for row in rows:
            for n in [_command_name(row[0]), *re.findall(r"`/([a-z0-9-]+\.[a-z0-9-]+)`", row[1])]:
                if n not in names:
                    names.append(n)
    if not names:
        raise ValueError("no built-in skill commands found")
    return names


def parse_subagents(text: str) -> list[str]:
    m = re.search(r"(?ms)^## Built-in Sub-Agents\s*$(.*?)(?=^## |\Z)", text)
    names = re.findall(r"(?m)^\* \*\*`([a-z][a-z0-9-]*)`\*\*:", m.group(1)) if m else []
    if not names:
        raise ValueError("no built-in sub-agents found")
    return names


def extract(ctx) -> dict:
    commands = ctx.get(COMMANDS)
    return {
        "command": parse_commands(commands),
        "skill": {"names": parse_skill_commands(commands)},
        "agent": {"names": parse_subagents(ctx.get(AGENTS))},
    }
