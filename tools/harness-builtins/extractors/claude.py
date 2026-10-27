"""Claude Code: built-in commands, bundled skills and built-in subagents from code.claude.com."""

import re

COMMANDS = "https://code.claude.com/docs/en/commands.md"
SUBAGENTS = "https://code.claude.com/docs/en/sub-agents.md"

_LIST = r"((?:`/[a-z0-9-]+`(?:, | and |,)?)+)"


def _cells(line: str) -> list[str]:
    # a pipe inside a code span is escaped as \| in the page
    return [c.strip() for c in re.split(r"(?<!\\)\|", line.strip())[1:-1]]


def _slashed(text: str) -> list[str]:
    return re.findall(r"`/([a-z0-9-]+)`", text)


def _rows(text: str) -> list[tuple[str, str]]:
    """(name, purpose) of every row of the `Command | Purpose` table.

    A row of subcommands (`/x consent`, `/x revoke`) of a command that has its own row is dropped,
    so it cannot change what that row says x is.
    """
    rows = []
    for line in text.splitlines():
        if not line.startswith("| `/"):
            continue
        cells = _cells(line)
        m = re.match(r"`/([a-z][a-z0-9-]*)( [a-z])?", cells[0])
        if not m or len(cells) < 2:
            raise ValueError(f"cannot read the command of {line[:80]!r}")
        rows.append((m.group(1), " | ".join(cells[1:]), m.group(2) is not None))
    if not rows:
        raise ValueError("no command rows found")
    own = {name for name, _, sub in rows if not sub}
    return [(name, purpose) for name, purpose, sub in rows if not (sub and name in own)]


def parse_commands(text: str) -> dict:
    """{"command": {names, aliases}, "skill": {names, aliases}}.

    A row marked **Skill** or **Workflow** is a bundled skill, any other row a built-in command.
    A row that starts "Alias for `/x`" or "Alias of [`/x`]" is only an alias of x. Aliases are
    named by "Alias: `/y`", "Aliases: `/y`, `/z`" and "`/y` and `/z` are aliases"; a sentence
    about an earlier version ("were aliases") does not count.
    """
    kinds, owners, pointing = {}, {}, {}
    for name, purpose in _rows(text):
        target = re.match(r"Alias (?:for|of) \[?`/([a-z0-9-]+)`", purpose)
        if target:
            pointing[name] = target.group(1)
            continue
        kinds[name] = "skill" if re.search(r"\*\*\[(Skill|Workflow)\]", purpose) else "command"
        found = []
        for m in re.finditer(r"\bAlias(?:es)?: " + _LIST, purpose):
            found += _slashed(m.group(1))
        for m in re.finditer(_LIST + r" are aliases\b", purpose):
            found += _slashed(m.group(1))
        if found:
            owners[name] = found
    for alias, target in pointing.items():
        if target not in kinds:
            raise ValueError(f"/{alias} is an alias of /{target}, which has no row")
        owners.setdefault(target, []).append(alias)
    out = {kind: {"names": [], "aliases": {}} for kind in ("command", "skill")}
    for name, kind in kinds.items():
        out[kind]["names"].append(name)
        found = sorted({a for a in owners.get(name, []) if a != name})
        if found:
            out[kind]["aliases"][name] = found
    for kind, got in out.items():
        if not got["names"]:
            raise ValueError(f"no {kind} found")
    return out


def parse_subagents(text: str) -> dict:
    """The built-in subagents: one per tab (or bold heading line) of the section, plus the rows of the Other table.

    "agent_override" holds those the page says a user or project subagent of the same name
    overrides ("named `X` overrides the built-in"); "agent" holds the rest. A tab title or
    heading is the name as written there (the pin spells `general-purpose` in lower case).
    """
    m = re.search(r"(?ms)^## Built-in subagents\s*$(.*?)(?=^## |\Z)", text)
    if not m:
        raise ValueError("section 'Built-in subagents' not found")
    section = m.group(1)
    titles = re.findall(r'<Tab title="([^"]+)">', section) or re.findall(r"(?m)^\*\*([^*\n]+)\*\*[ \t]*$", section)
    names = [t for t in titles if t != "Other"]
    table = re.search(r"(?m)^[ \t]*\| Agent \|.*\n[ \t]*\|[ :|-]+\n((?:[ \t]*\|.*\n?)+)", section)
    if table:
        names += [_cells(line.strip())[0] for line in table.group(1).splitlines() if line.strip().startswith("|")]
    if not names:
        raise ValueError("no built-in subagents found")
    override = re.findall(r"named `([^`]+)` overrides the built-in", section)
    missing = [o for o in override if o not in names]
    if missing:
        raise ValueError(f"overridable subagents without a tab or row: {missing}")
    return {
        "agent_override": override,
        "agent": [n for n in names if n not in override],
    }


def extract(ctx) -> dict:
    cmds = parse_commands(ctx.get(COMMANDS))
    return {
        "command": cmds["command"],
        "skill": cmds["skill"],
        **{k: {"names": v} for k, v in parse_subagents(ctx.get(SUBAGENTS)).items()},
    }
