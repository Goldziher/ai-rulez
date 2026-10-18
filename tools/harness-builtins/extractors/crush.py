"""Crush: command-palette commands, embedded skills and default agents at the pinned tag."""

import re

from hb import fetch

COMMANDS = "internal/ui/dialog/commands.go"
CONFIG = "internal/config/config.go"
SKILLS = "internal/skills/builtin/"


def parse_commands(text: str) -> dict:
    """Ids of the palette items built from a string literal (`NewCommandItem(styles, "id", ...)`) and
    the aliases of `.WithAliases("a", "b")` on the same statement."""
    names, aliases = [], {}
    for line in text.splitlines():
        m = re.search(r'NewCommandItem\(\s*[\w.]+,\s*"([^"]+)"\s*,', line)
        if not m:
            continue
        names.append(m.group(1))
        al = re.search(r"\.WithAliases\(([^)]*)\)", line)
        if al:
            aliases[m.group(1)] = re.findall(r'"([^"]+)"', al.group(1))
    if not names:
        raise ValueError("no palette commands found")
    return {"names": names, "aliases": aliases}


def parse_agents(text: str) -> list[str]:
    """Ids of the agents SetupAgents puts in the agents map (keys are Agent* constants)."""
    consts = dict(re.findall(r'\b(Agent\w+)\s+string\s*=\s*"([^"]+)"', text))
    m = re.search(r"func \(c \*Config\) SetupAgents\(\)\s*\{", text)
    if not m:
        raise ValueError("SetupAgents not found")
    end = text.find("\n\tc.Agents", m.end())
    if end < 0:
        raise ValueError("SetupAgents does not assign c.Agents")
    keys = re.findall(r"(?m)^\t\t(\w+):\s*\{$", text[m.end() : end])
    unknown = [k for k in keys if k not in consts]
    if not keys or unknown:
        raise ValueError(f"agent keys that are not constants: {unknown or 'no agents found'}")
    return [consts[k] for k in keys]


def extract(ctx) -> dict:
    skills = []
    for path in ctx.tree(SKILLS):
        if path.endswith("/SKILL.md") and path.count("/") == SKILLS.count("/") + 1:
            skills.append(fetch.frontmatter_field(ctx.raw(path), "name") or path.split("/")[-2])
    if not skills:
        raise ValueError("no embedded skills found")
    return {
        "command": parse_commands(ctx.raw(COMMANDS)),
        "skill": {"names": skills},
        "agent": {"names": parse_agents(ctx.raw(CONFIG))},
    }
