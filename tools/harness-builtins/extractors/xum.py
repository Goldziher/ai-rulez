"""Xum (coder/mux): slash commands, built-in skills and built-in agents at the pinned commit."""

import re

REGISTRY = "src/browser/utils/slashCommands/registry.ts"
SKILLS = "src/node/builtinSkills/"
AGENTS = "src/node/builtinAgents/"
AGENT_LIST = "src/node/services/agentDefinitions/builtInAgentDefinitions.ts"


def strip_comments(text: str) -> str:
    return re.sub(r"(?m)^\s*//.*$", "", text)


def parse_commands(text: str) -> list[str]:
    """Top-level keys of SLASH_COMMAND_DEFINITIONS (sub-commands such as `/plan open` are not commands)."""
    text = strip_comments(text)
    single = dict(re.findall(r'const (\w+): SlashCommandDefinition = \{\s*key: "([^"]+)"', text))
    many = {
        name: re.findall(r'"([^"]+)"', arr)
        for name, arr in re.findall(r"const (\w+): SlashCommandDefinition\[\] = \(\[([^\]]*)\] as const\)\.map", text)
    }
    m = re.search(r"export const SLASH_COMMAND_DEFINITIONS\b[^=]*=\s*\[(.*?)\];", text, re.DOTALL)
    if not m:
        raise ValueError("SLASH_COMMAND_DEFINITIONS not found")
    names = []
    for item in re.findall(r"(\.\.\.)?(\w+)", m.group(1)):
        spread, ident = item
        if spread and ident in many:
            names += many[ident]
        elif not spread and ident in single:
            names.append(single[ident])
        else:
            raise ValueError(f"cannot resolve {ident!r} in SLASH_COMMAND_DEFINITIONS")
    return names


def parse_agent_ids(text: str) -> list[str]:
    """The ids of BUILT_IN_SOURCES (an agent's id is its file name; the front-matter name is a display title)."""
    m = re.search(r"const BUILT_IN_SOURCES\b[^=]*=\s*\[(.*?)\n\];", strip_comments(text), re.DOTALL)
    ids = re.findall(r'\bid:\s*"([^"]+)"', m.group(1)) if m else []
    if not ids:
        raise ValueError("no built-in agents found")
    return ids


def markdown_stems(ctx, prefix: str) -> list[str]:
    """File stems of the *.md files directly in prefix (a skill's or agent's id)."""
    return [p[len(prefix) : -3] for p in ctx.tree(prefix) if p.endswith(".md") and "/" not in p[len(prefix) :]]


def extract(ctx) -> dict:
    skills = markdown_stems(ctx, SKILLS)
    if not skills:
        raise ValueError("no built-in skills found")
    return {
        "command": {"names": parse_commands(ctx.raw(REGISTRY))},
        "skill": {"names": skills},
        "agent": {"names": parse_agent_ids(ctx.raw(AGENT_LIST))},
    }
