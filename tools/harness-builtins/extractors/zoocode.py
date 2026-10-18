"""Zoo Code: built-in slash commands and default modes at the pinned tag."""

import re

COMMANDS = "src/services/command/built-in-commands.ts"
MODES = "packages/types/src/mode.ts"


def parse_commands(text: str) -> list[str]:
    """Keys of BUILT_IN_COMMANDS (a command is looked up by its key)."""
    m = re.search(r"\bBUILT_IN_COMMANDS\b[^=\n]*=\s*\{\n", text)
    end = text.find("\n}\n", m.end()) if m else -1
    if end < 0:
        raise ValueError("BUILT_IN_COMMANDS not found")
    names = [a or b for a, b in re.findall(r"""(?m)^\t(?:(\w[\w-]*)|"([^"]+)")\s*:\s*\{\s*$""", text[m.end() : end])]
    if not names:
        raise ValueError("no built-in commands found")
    return names


def parse_modes(text: str) -> list[str]:
    """Slugs of DEFAULT_MODES."""
    m = re.search(r"\bDEFAULT_MODES\b[^=\n]*=\s*\[\n", text)
    end = text.find("\n]", m.end()) if m else -1
    if end < 0:
        raise ValueError("DEFAULT_MODES not found")
    slugs = re.findall(r"""(?m)^\t\tslug:\s*["']([^"']+)["']""", text[m.end() : end])
    if not slugs:
        raise ValueError("no default modes found")
    return slugs


def extract(ctx) -> dict:
    return {
        "command": {"names": parse_commands(ctx.raw(COMMANDS))},
        "agent": {"names": parse_modes(ctx.raw(MODES))},
    }
