"""MiMo Code: built-in commands (the Default table and the TUI slash commands), bundled skills and
native agents at the pinned commit."""

import re

from hb import fetch

COMMANDS = "packages/cli/src/command/index.ts"
TUI = "packages/cli/src/cli/cmd/tui/"
# the files that declare `slash: { name, aliases }` entries; the count check below catches a new one
TUI_FILES = ("app.tsx", "component/prompt/index.tsx", "routes/session/index.tsx")
SKILLS = "packages/cli/src/skill/builtin/.bundle/"
AGENTS = "packages/cli/src/agent/agent.ts"

SLASH = re.compile(r'\bslash:\s*\{\s*name:\s*"([^"]+)"(?:,\s*aliases:\s*\[([^\]]*)\])?')


def strip_comments(text: str) -> str:
    return re.sub(r"(?m)^\s*//.*$", "", text)


def parse_defaults(text: str) -> list[str]:
    """Names of the commands index.ts registers from its Default table (some only behind an experimental flag)."""
    text = strip_comments(text)
    m = re.search(r"export const Default = \{(.*?)\} as const", text, re.DOTALL)
    if not m:
        raise ValueError("Default table not found")
    table = dict(re.findall(r'(\w+):\s*"([^"]+)"', m.group(1)))
    used = re.findall(r"commands\[Default\.(\w+)\]\s*=", text)
    if not used:
        raise ValueError("no commands[Default.X] assignments found")
    return [table[k] for k in dict.fromkeys(used)]


def parse_slash(text: str) -> dict[str, list[str]]:
    """{name: [aliases]} of the `slash:` entries of one TUI source file."""
    text = strip_comments(text)
    found = {m.group(1): re.findall(r'"([^"]+)"', m.group(2) or "") for m in SLASH.finditer(text)}
    if len(found) != len(re.findall(r"\bslash:\s*\{", text)):
        raise ValueError("a slash: entry has a shape the parser does not read")
    return found


def parse_agents(text: str) -> list[str]:
    """Native agents: the `name:` of each entry of the `agents` table (opt-in entries sit deeper and are skipped)."""
    m = re.search(r"const agents: Record<string, Info> = \{(.*?)\n        \}\n", text, re.DOTALL)
    if not m:
        raise ValueError("agents table not found")
    body = strip_comments(m.group(1))
    names = re.findall(r'^( +)name: "([^"]+)",\s*$', body, re.MULTILINE)
    if not names:
        raise ValueError("no agents found")
    top = min(len(i) for i, _ in names)
    return [n for i, n in names if len(i) == top]


def extract(ctx) -> dict:
    commands = parse_defaults(ctx.raw(COMMANDS))
    slash = {}
    for f in TUI_FILES:
        slash.update(parse_slash(ctx.raw(TUI + f)))
    skills = []
    for path in ctx.tree(SKILLS):
        if path.endswith("/SKILL.md") and path.count("/") == SKILLS.count("/") + 1:
            skills.append(fetch.frontmatter_field(ctx.raw(path), "name") or path.split("/")[-2])
    return {
        "command": {"names": commands},
        "tui": {"names": list(slash), "aliases": {n: a for n, a in slash.items() if a}},
        "skill": {"names": skills},
        "agent": {"names": parse_agents(ctx.raw(AGENTS))},
    }
