"""Zed: the native slash commands of the agent and its built-in skills at the pinned commit."""

import re

AGENT = "crates/agent/src/agent.rs"
SKILLS = "crates/agent_skills/agent_skills.rs"


def strip_comments(text: str) -> str:
    return re.sub(r"(?m)^\s*//.*$", "", text)


def parse_commands(text: str) -> list[str]:
    """Native commands: the `*_COMMAND_NAME` constants of agent.rs (each is offered as an AvailableCommand)."""
    names = re.findall(r'(?m)^pub const [A-Z_]+_COMMAND_NAME: &str = "([^"]+)";', strip_comments(text))
    if not names:
        raise ValueError("no *_COMMAND_NAME constants found")
    return names


def parse_skills(text: str) -> list[str]:
    """The names of BUILTIN_SKILL_ENTRIES, the embedded skills."""
    m = re.search(r"const BUILTIN_SKILL_ENTRIES\b[^=]*=\s*&\[(.*?)\];", strip_comments(text), re.DOTALL)
    names = re.findall(r'\(\s*"([^"]+)"\s*,', m.group(1)) if m else []
    if not names:
        raise ValueError("BUILTIN_SKILL_ENTRIES not found")
    return names


def extract(ctx) -> dict:
    return {
        "command": {"names": parse_commands(ctx.raw(AGENT))},
        "skill": {"names": parse_skills(ctx.raw(SKILLS))},
    }
