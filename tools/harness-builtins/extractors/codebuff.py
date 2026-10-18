"""Codebuff: slash commands of the CLI (the Codebuff and Freebuff builds together) at the pinned commit."""

import re

SLASH = "cli/src/data/slash-commands.ts"
CONSTANTS = "cli/src/utils/constants.ts"

ENTRY = re.compile(r"^  \{\n(.*?)^  \},", re.S | re.M)


def strip_comments(text: str) -> str:
    return re.sub(r"(?m)^\s*//.*$", "", text)


def parse_commands(text: str) -> dict[str, list[str]]:
    """{id: [aliases]} of the literal entries of ALL_SLASH_COMMANDS.

    Both builds are listed: FREEBUFF_REMOVED_COMMAND_IDS / FREEBUFF_ONLY_COMMAND_IDS only filter them.
    """
    m = re.search(r"const ALL_SLASH_COMMANDS: SlashCommand\[\] = \[\n(.*?)\n\]\n", strip_comments(text), re.DOTALL)
    if not m:
        raise ValueError("ALL_SLASH_COMMANDS not found")
    out = {}
    for e in ENTRY.finditer(m.group(1) + "\n"):
        ident = re.search(r"^    id: '([^']+)',", e.group(1), re.MULTILINE)
        if not ident:
            raise ValueError("a command entry has no literal id")
        al = re.search(r"^    aliases: \[([^\]]*)\],", e.group(1), re.MULTILINE)
        out[ident.group(1)] = re.findall(r"'([^']+)'", al.group(1)) if al else []
    if not out:
        raise ValueError("no slash commands found")
    return out


def parse_modes(slash: str, constants: str) -> dict[str, list[str]]:
    """`mode:<name>` commands (alias `model:<name>`), generated from the keys of AGENT_MODE_TO_ID."""
    template = re.search(r"id: `mode:\$\{mode\.toLowerCase\(\)\}`", slash) and re.search(
        r"aliases: \[`model:\$\{mode\.toLowerCase\(\)\}`\]", slash
    )
    if not template:
        raise ValueError("the MODE_COMMANDS template changed")
    m = re.search(r"export const AGENT_MODE_TO_ID = \{(.*?)\} as const", strip_comments(constants), re.DOTALL)
    keys = re.findall(r"^  ([A-Z_]+):", m.group(1), re.MULTILINE) if m else []
    if not keys:
        raise ValueError("AGENT_MODE_TO_ID not found")
    return {f"mode:{k.lower()}": [f"model:{k.lower()}"] for k in keys}


def extract(ctx) -> dict:
    slash = ctx.raw(SLASH)
    commands = parse_commands(slash)
    modes = parse_modes(slash, ctx.raw(CONSTANTS))
    clash = set(commands) & set(modes)
    if clash:
        raise ValueError(f"mode commands repeat literal ones: {sorted(clash)}")
    commands.update(modes)
    return {"command": {"names": list(commands), "aliases": {n: a for n, a in commands.items() if a}}}
