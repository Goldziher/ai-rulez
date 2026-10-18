"""Qwen Code: slash commands, bundled skills and built-in subagents at the pinned tag.

Qwen Code is a fork of Gemini CLI; the command loader is parsed by extractors/gemini.py,
which this file loads by path.
"""

import importlib.util
import re
from pathlib import Path

_spec = importlib.util.spec_from_file_location("hb_extractor_gemini", Path(__file__).with_name("gemini.py"))
gemini = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(gemini)

LOADER = "packages/cli/src/services/BuiltinCommandLoader.ts"
SKILLS = "packages/core/src/skills/bundled/"
AGENTS = "packages/core/src/subagents/builtin-agents.ts"

# `/ide` is built outside the allDefinitions array (it is awaited and may fail), as
# `resolvedIdeCommand = await ideCommand()`.
SEPARATE = re.compile(r"=\s*await\s+(\w+Command)\(\)")


def parse_agents(text: str) -> list[str]:
    """The name of each entry of BuiltinAgentRegistry.BUILTIN_AGENTS (an object whose first key is name)."""
    m = re.search(r"\bBUILTIN_AGENTS\b[^=]*=\s*\[", text)
    end = text.find("\n  ];", m.end()) if m else -1
    if end < 0:
        raise ValueError("BUILTIN_AGENTS array not found")
    names = []
    for e in re.finditer(r"(?m)^\s*\{\s*\n\s*name:\s*(?:'([^']*)'|\"([^\"]*)\"|([A-Za-z_]\w*))", text[m.end() : end]):
        if e.group(3):
            names.append(gemini.resolve_constant(text, e.group(3)))
        else:
            names.append(e.group(1) or e.group(2))
    if not names:
        raise ValueError("no built-in subagents found")
    return names


def extract(ctx) -> dict:
    text = ctx.raw(LOADER)
    extra = tuple(SEPARATE.findall(text))
    return {
        "command": gemini.commands(ctx, LOADER, extra),
        "skill": gemini.skills(ctx, SKILLS),
        "agent": {"names": parse_agents(ctx.raw(AGENTS))},
    }
