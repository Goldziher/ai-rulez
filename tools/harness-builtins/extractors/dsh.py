"""DeepSeek Harness (dsh): the skills its packages bundle, at the pinned commit.

Slash commands are registered by many plugin packages (`ctx.commands.register({ name: ... })` in
command-compact, command-goal, plan-mode, ...), with no central table to read, so they stay
hand-maintained in the data file.
"""

import re

OFFICE = "packages/skill/skill-office/"
OFFICE_SRC = OFFICE + "src/index.ts"
ACL = "packages/sandbox/sandbox-windows-acl/"
ACL_SRC = ACL + "src/acl-skill.ts"


def parse_office(text: str) -> list[str]:
    """The SKILL_NAMES tuple of skill-office."""
    m = re.search(r"const SKILL_NAMES\s*=\s*\[(.*?)\]\s*as const", text, re.DOTALL)
    names = re.findall(r"'([^']+)'", m.group(1)) if m else []
    if not names:
        raise ValueError("SKILL_NAMES not found")
    return names


def parse_acl(text: str) -> str:
    """The ACL_DIAGNOSIS_SKILL constant of sandbox-windows-acl."""
    m = re.search(r"export const ACL_DIAGNOSIS_SKILL\s*=\s*'([^']+)'", text)
    if not m:
        raise ValueError("ACL_DIAGNOSIS_SKILL not found")
    return m.group(1)


def extract(ctx) -> dict:
    tree = set(ctx.tree("packages/"))
    found = {
        "office": [(n, OFFICE) for n in parse_office(ctx.raw(OFFICE_SRC))],
        "acl": [(parse_acl(ctx.raw(ACL_SRC)), ACL)],
    }
    out = {}
    for key, entries in found.items():
        for name, base in entries:
            if f"{base}assets/{name}/SKILL.md" not in tree:
                raise ValueError(f"skill {name!r} has no {base}assets/{name}/SKILL.md")
        out[key] = {"names": [n for n, _ in entries]}
    return out
