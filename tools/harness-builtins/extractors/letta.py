"""Letta Code: slash commands, bundled skills and built-in sub-agents at the pinned tag."""

import re

from hb import fetch

REGISTRY = "src/cli/commands/registry.ts"
SKILLS = "src/skills/builtin/"
SUBAGENTS = "src/agent/subagents/builtin/"


def parse_commands(text: str) -> list[str]:
    """Keys of `export const commands`. Hidden entries (aliases, deprecated commands) still run."""
    m = re.search(r"export const commands\b[^=]*=\s*\{(.*?)^\};", text, re.DOTALL | re.MULTILINE)
    if not m:
        raise ValueError("export const commands not found")
    body = re.sub(r"(?m)^\s*//.*$", "", m.group(1))
    names = re.findall(r'^  "/([^"]+)":\s*\{', body, re.MULTILINE)
    if not names:
        raise ValueError("no commands found")
    return names


def markdown_names(ctx, prefix: str, fields: tuple[str, ...], skill_dirs: bool = False) -> list[str]:
    """Names of the markdown files directly in prefix (skill_dirs: of <prefix><dir>/SKILL.md): the first
    front-matter field of `fields` that is set, else the directory or the file stem."""
    names = []
    depth = prefix.count("/") + (1 if skill_dirs else 0)
    for path in ctx.tree(prefix):
        if path.count("/") != depth or not path.endswith("/SKILL.md" if skill_dirs else ".md"):
            continue
        text = ctx.raw(path)
        found = next((v for v in (fetch.frontmatter_field(text, f) for f in fields) if v), None)
        if not found:
            parts = path.split("/")
            found = parts[-2] if parts[-1] == "SKILL.md" else parts[-1][:-3]
        names.append(found)
    return list(dict.fromkeys(names))


def extract(ctx) -> dict:
    skills = markdown_names(ctx, SKILLS, ("id", "name"), skill_dirs=True)
    agents = markdown_names(ctx, SUBAGENTS, ("name",))
    if not skills or not agents:
        raise ValueError("no skills or sub-agents found")
    return {
        "command": {"names": parse_commands(ctx.raw(REGISTRY))},
        "skill": {"names": skills},
        "agent": {"names": agents},
    }
