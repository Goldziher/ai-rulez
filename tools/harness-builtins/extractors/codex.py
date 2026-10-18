"""Codex: slash commands, bundled sample skills and built-in agent roles at the pinned tag."""

import re

from hb import fetch, rustenum

SLASH = "codex-rs/tui/src/slash_command.rs"
ROLES = "codex-rs/core/src/agent/role.rs"
SAMPLES = "codex-rs/skills/src/assets/samples/"


def parse_roles(text: str) -> list[str]:
    """Names of the entries of the built_in::configs map (commented-out entries do not count)."""
    body = text[text.index("mod built_in") :]
    body = re.sub(r"(?m)^\s*//.*$", "", body)
    names = []
    for m in re.finditer(r'\(\s*(DEFAULT_ROLE_NAME|"[a-z0-9_-]+")\.to_string\(\),\s*AgentRoleConfig', body):
        names.append("default" if m.group(1) == "DEFAULT_ROLE_NAME" else m.group(1).strip('"'))
    if not names:
        raise ValueError("no built-in roles found")
    return names


def extract(ctx) -> dict:
    skills = []
    for path in ctx.tree(SAMPLES):
        if path.endswith("/SKILL.md") and path.count("/") == SAMPLES.count("/") + 1:
            name = fetch.frontmatter_field(ctx.raw(path), "name")
            skills.append(name or path.split("/")[-2])
    return {
        "command": rustenum.commands(ctx.raw(SLASH), "SlashCommand"),
        "skill": {"names": skills},
        "agent": {"names": parse_roles(ctx.raw(ROLES))},
    }
