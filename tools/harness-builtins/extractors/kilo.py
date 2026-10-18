"""Kilo: slash commands, the built-in skill and the native agents at the pinned tag.

Kilo is a fork of OpenCode; the parsing helpers of extractors/opencode.py are loaded by path.
Kilo adds its own commands (kilocode/command), TUI slash commands (kilocode/kilo-commands.tsx,
kilocode/plugins, kilocode/cli/cmd/tui) and agents (kilocode/agent patchAgents).
"""

import importlib.util
import re
from pathlib import Path

_spec = importlib.util.spec_from_file_location("hb_extractor_opencode", Path(__file__).with_name("opencode.py"))
oc = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(oc)

COMMANDS = "packages/opencode/src/command/index.ts"
RESERVED = "packages/opencode/src/kilocode/command/reserved.ts"
SKILLS = "packages/opencode/src/kilocode/skills/builtin.ts"
AGENTS = "packages/opencode/src/agent/agent.ts"
PATCH = "packages/opencode/src/kilocode/agent/index.ts"
TUI_DIRS = ("packages/tui/src/", "packages/opencode/src/kilocode/")


def parse_registered(text: str) -> list[str]:
    """Commands the command service registers itself: `commands[Default.X]`, `commands.x` and `commands["x"]`."""
    text = oc.strip_comments(text)
    names = oc.parse_defaults(text)
    for m in re.finditer(r"""\bcommands(?:\.(\w+)|\["([^"]+)"\])\s*=\s*[^=]""", text):
        name = m.group(1) or m.group(2)
        if name not in names:
            names.append(name)
    return names


def parse_reserved(text: str) -> list[str]:
    """Names `reserved()` refuses for a user command: `return name === "goal"`."""
    m = re.search(r"\bfunction reserved\([^)]*\)\s*\{(.*?)\n\}", oc.strip_comments(text), re.DOTALL)
    names = re.findall(r"""name\s*===\s*["']([^"']+)["']""", m.group(1)) if m else []
    if not names:
        raise ValueError("reserved command names not found")
    return names


def parse_builtin_skills(text: str) -> list[str]:
    """Entries of the BUILTIN_SKILLS array: objects whose first key is name."""
    text = oc.strip_comments(text)
    m = re.search(r"\bBUILTIN_SKILLS\b[^=]*=\s*\[", text)
    if not m:
        raise ValueError("BUILTIN_SKILLS not found")
    names = [a or b for a, b in re.findall(r"(?m)^\s*\{\s*\n\s*name:\s*" + oc.STR, text[m.end() :])]
    if not names:
        raise ValueError("no built-in skills found")
    return names


def apply_patch(names: list[str], text: str) -> list[str]:
    """Agents after patchAgents: `agents.old` renamed to `agents.new` (and deleted) and agents added."""
    text = oc.strip_comments(text)
    m = re.search(r"\bfunction patchAgents\b", text)
    if not m:
        raise ValueError("patchAgents not found")
    body = text[m.end() :]
    out = list(names)
    for new, old in re.findall(r"agents\.(\w+)\s*=\s*\{\s*\.\.\.agents\.(\w+),", body):
        if f"delete agents.{old}" in body and old in out:
            out[out.index(old)] = new
    for _key, name in re.findall(r"agents\.(\w+)\s*=\s*\{\s*name:\s*\"([^\"]+)\"", body):
        if name not in out:
            out.append(name)
    return out


def extract(ctx) -> dict:
    registered = parse_registered(ctx.raw(COMMANDS))
    reserved = parse_reserved(ctx.raw(RESERVED))
    missing = [n for n in reserved if n not in registered]
    if missing:
        raise ValueError(f"reserved names that are not registered commands: {missing}")
    tui = {"names": [], "aliases": {}}
    for d in TUI_DIRS:
        got = oc.slash_commands(ctx, d)
        tui["names"] += [n for n in got["names"] if n not in tui["names"]]
        for name, al in got["aliases"].items():
            tui["aliases"][name] = sorted(set(tui["aliases"].get(name, [])) | set(al))
    return {
        "command_default": {"names": [n for n in registered if n not in reserved]},
        "command_reserved": {"names": reserved},
        "command_tui": tui,
        "skill": {"names": parse_builtin_skills(ctx.raw(SKILLS))},
        "agent": {"names": apply_patch(oc.parse_native_agents(ctx.raw(AGENTS)), ctx.raw(PATCH))},
    }
