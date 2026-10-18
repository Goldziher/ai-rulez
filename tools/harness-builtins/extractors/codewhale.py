"""CodeWhale: slash commands (split by whether a workspace command may shadow them), the bundled
skill pack and the sub-agent roles at the pinned tag."""

import json
import re

from hb import rustenum

GROUPS = "crates/tui/src/commands/groups/"
USER_REGISTRY = "crates/tui/src/commands/user_registry.rs"
MATRIX = "crates/tui/assets/skills-catalog-matrix.json"
SUBAGENTS = "docs/SUBAGENTS.md"

# CommandInfo { name: "x", aliases: &[...] } and the debug group's registration!(Ty, "x", &[...], ...)
INFO = re.compile(r'\bname:\s*"([^"]+)",\s*aliases:\s*&\[([^\]]*)\]')
MACRO = re.compile(r'\bregistration!\(\s*\w+,\s*"([^"]+)",\s*&\[([^\]]*)\]')


def is_source(path: str) -> bool:
    return path.endswith(".rs") and not re.search(r"test|fixture", path)


def parse_commands(text: str) -> dict[str, list[str]]:
    """{name: [aliases]} of the command descriptors in one source file (comments do not count)."""
    text = rustenum.strip_comments(text)
    out = {}
    for m in list(INFO.finditer(text)) + list(MACRO.finditer(text)):
        out[m.group(1)] = re.findall(r'"([^"]*)"', m.group(2))
    return out


def parse_protected(text: str) -> list[str]:
    """The names of the PROTECTED_BUILTINS array: built-ins a workspace command may never replace."""
    m = re.search(r"const PROTECTED_BUILTINS:\s*&\[&str\]\s*=\s*&\[(.*?)\];", rustenum.strip_comments(text), re.DOTALL)
    if not m:
        raise ValueError("PROTECTED_BUILTINS not found")
    names = re.findall(r'"([^"]+)"', m.group(1))
    if not names:
        raise ValueError("PROTECTED_BUILTINS is empty")
    return names


def split_commands(commands: dict[str, list[str]], protected: list[str]) -> tuple[dict, dict]:
    """(protected, shadowable) as extractor results; a protected name absent from the registry is dropped."""
    prot = {n: a for n, a in commands.items() if n in protected}
    rest = {n: a for n, a in commands.items() if n not in protected}

    def result(d):
        return {"names": list(d), "aliases": {n: a for n, a in d.items() if a}}

    return result(prot), result(rest)


def parse_skills(text: str) -> list[str]:
    """The skills of the bundled-pack matrix, which the code keeps in bijection with BUNDLED_SKILLS."""
    names = [s["name"] for s in json.loads(text)["skills"]]
    if not names:
        raise ValueError("no bundled skills found")
    return names


def table_rows(text: str, header: str) -> list[list[str]]:
    """The cells of the body rows of the markdown table whose first header cell is `header`."""
    lines, rows, on = text.splitlines(), [], False
    for line in lines:
        cells = [c.strip() for c in line.strip().strip("|").split("|")] if line.lstrip().startswith("|") else None
        if cells is None:
            if on:
                break
            continue
        if cells[0] == header:
            on = True
        elif on and not set("".join(cells)) <= set("-: "):
            rows.append(cells)
    if not rows:
        raise ValueError(f"no table with the header {header!r}")
    return rows


def parse_roles(text: str) -> dict:
    """Sub-agent roles (the Role table) and their aliases (the Canonical/Aliases table)."""
    roles = [re.sub(r"[`]", "", r[0]) for r in table_rows(text, "Role")]
    aliases = {}
    for row in table_rows(text, "Canonical"):
        name = row[0].strip("`")
        found = re.findall(r"`([^`]+)`", re.sub(r"\([^)]*\)", "", row[1]))
        if name in roles and found:
            aliases[name] = found
    return {"names": roles, "aliases": aliases}


def extract(ctx) -> dict:
    commands = {}
    for path in ctx.tree(GROUPS):
        if is_source(path):
            commands.update(parse_commands(ctx.raw(path)))
    protected, rest = split_commands(commands, parse_protected(ctx.raw(USER_REGISTRY)))
    return {
        "command_protected": protected,
        "command": rest,
        "skill": {"names": parse_skills(ctx.raw(MATRIX))},
        "agent": parse_roles(ctx.raw(SUBAGENTS)),
    }
