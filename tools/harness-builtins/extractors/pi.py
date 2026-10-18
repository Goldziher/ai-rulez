"""Pi: built-in slash commands and the commands of the built-in extensions at the pinned tag."""

import posixpath
import re

SLASH = "packages/coding-agent/src/core/slash-commands.ts"
EXTENSIONS = "packages/coding-agent/src/extensions/index.ts"

STR = r"""(?:"([^"\n]*)"|'([^'\n]*)')"""


def strip_comments(text: str) -> str:
    return re.sub(r"(?m)^[ \t]*//.*$", "", text)


def parse_builtin_commands(text: str) -> list[str]:
    """Names of BUILTIN_SLASH_COMMANDS."""
    text = strip_comments(text)
    m = re.search(r"\bBUILTIN_SLASH_COMMANDS\b[^=]*=\s*\[", text)
    end = text.find("\n];", m.end()) if m else -1
    if end < 0:
        raise ValueError("BUILTIN_SLASH_COMMANDS not found")
    names = [a or b for a, b in re.findall(r"\{\s*name:\s*" + STR, text[m.end() : end])]
    if not names:
        raise ValueError("no built-in slash commands found")
    return names


def parse_builtin_extensions(text: str) -> list[str]:
    """Module paths (relative to the extensions index) of the extensions in builtInExtensions."""
    text = strip_comments(text)
    imports = {ident: path for ident, path in re.findall(r"import\s+(\w+)\s+from\s+\"([^\"]+)\"", text)}
    m = re.search(r"\bbuiltInExtensions\b[^=]*=\s*\[", text)
    end = text.find("\n];", m.end()) if m else -1
    if end < 0:
        raise ValueError("builtInExtensions not found")
    factories = re.findall(r"\bfactory:\s*(\w+)", text[m.end() : end])
    missing = [f for f in factories if f not in imports]
    if not factories or missing:
        raise ValueError(f"built-in extension factories without an import: {missing or 'none found'}")
    return [imports[f] for f in factories]


def parse_registered_commands(text: str) -> list[str]:
    return [a or b for a, b in re.findall(r"\bregisterCommand\(\s*" + STR, strip_comments(text))]


def extract(ctx) -> dict:
    names = parse_builtin_commands(ctx.raw(SLASH))
    for module in parse_builtin_extensions(ctx.raw(EXTENSIONS)):
        path = posixpath.normpath(posixpath.join(posixpath.dirname(EXTENSIONS), module))
        names += [n for n in parse_registered_commands(ctx.raw(path)) if n not in names]
    return {"command": {"names": names}}
