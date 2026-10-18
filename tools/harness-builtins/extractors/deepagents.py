"""Deep Agents Code: slash commands from the generated catalog, bundled skills and the default subagent."""

import re

from hb import fetch

CATALOG = "libs/code/COMMANDS.md"
SKILLS = "libs/code/deepagents_code/built_in_skills/"
SUBAGENTS = "libs/deepagents/deepagents/middleware/subagents.py"


def _ticked(cell: str) -> list[str]:
    return [m.removeprefix("/") for m in re.findall(r"`(/[^`\s]+)`", cell)]


def parse_catalog(text: str) -> dict:
    """The public commands (table rows, with their aliases) and the hidden ones (list items).

    The counts in the `## Public (N)` and `## Hidden (N)` headings must match what was read.
    """
    names, aliases = [], {}
    sections = re.split(r"(?m)^## ", text)
    seen = {}
    for sec in sections[1:]:
        head = re.match(r"(Public|Hidden) \((\d+)\)", sec)
        if not head:
            continue
        found = []
        for line in sec.splitlines():
            if head.group(1) == "Public" and line.startswith("| `/"):
                cells = [c.strip() for c in line.strip().strip("|").split("|")]
                cmd = _ticked(cells[0])
                if len(cmd) != 1:
                    raise ValueError(f"cannot read the command of {line!r}")
                found.append(cmd[0])
                if len(cells) > 1 and _ticked(cells[1]):
                    aliases[cmd[0]] = _ticked(cells[1])
            elif head.group(1) == "Hidden" and (m := re.match(r"- `/([^`\s]+)`\s*$", line)):
                found.append(m.group(1))
        if len(found) != int(head.group(2)):
            raise ValueError(f"{head.group(1)}: heading says {head.group(2)} commands, read {len(found)}")
        seen[head.group(1)] = found
        names += found
    if "Public" not in seen:
        raise ValueError("no '## Public (N)' section found")
    return {"names": names, "aliases": aliases}


def parse_default_subagent(text: str) -> str:
    m = re.search(r'GENERAL_PURPOSE_SUBAGENT\s*:\s*SubAgent\s*=\s*\{\s*"name"\s*:\s*"([^"]+)"', text)
    if not m:
        raise ValueError("GENERAL_PURPOSE_SUBAGENT not found")
    return m.group(1)


def extract(ctx) -> dict:
    skills = []
    for path in ctx.tree(SKILLS):
        rest = path[len(SKILLS) :]
        if rest.endswith("/SKILL.md") and rest.count("/") == 1:
            skills.append(fetch.frontmatter_field(ctx.raw(path), "name") or rest.split("/")[0])
    return {
        "command": parse_catalog(ctx.raw(CATALOG)),
        "skill": {"names": skills},
        "agent": {"names": [parse_default_subagent(ctx.raw(SUBAGENTS))]},
    }
