"""OpenClaw: core slash commands (the built-in registry), the bundled plugin commands the slash-commands
page documents, and the bundled skills at the pinned tag."""

import re

from hb import fetch

REGISTRY = "src/auto-reply/commands-registry.shared.ts"
DOCS = "docs/tools/slash-commands.md"
SKILLS = "skills/"


def parse_registry(text: str) -> dict:
    """Core commands: the first argument of each defineBuiltinCommand(...) call, and the aliases its
    textAliases / nativeAliases options add."""
    names, aliases = [], {}
    text = re.sub(r"(?m)^\s*//.*$", "", text)
    calls = [m.start() for m in re.finditer(r"\bdefineBuiltinCommand\(\s*\"", text)]
    for start, end in zip(calls, calls[1:] + [len(text)], strict=True):
        call = text[start:end]
        key = re.match(r'defineBuiltinCommand\(\s*"([^"]+)"', call).group(1)
        found = []
        for opt in ("textAliases", "nativeAliases"):
            m = re.search(opt + r"\s*:\s*\[([^\]]*)\]", call)
            if m:
                found += [a.lstrip("/") for a in re.findall(r'"([^"]+)"', m.group(1))]
        names.append(key)
        keep = [a for a in dict.fromkeys(found) if a != key]
        if keep:
            aliases[key] = keep
    if not names:
        raise ValueError("no defineBuiltinCommand calls found")
    return {"names": names, "aliases": aliases}


def parse_plugin_commands(text: str) -> list[str]:
    """The commands in the first column of the 'Bundled plugin commands' table."""
    m = re.search(r"^#+ Bundled plugin commands\s*\n(.*?)(?=^#|\Z)", text, re.DOTALL | re.MULTILINE)
    if not m:
        raise ValueError("no 'Bundled plugin commands' section")
    names = []
    for line in m.group(1).splitlines():
        if line.startswith("|"):
            first = re.search(r"`/([a-z][a-z0-9-]*)", line.split("|")[1])
            if first:
                names.append(first.group(1))
    if not names:
        raise ValueError("no commands in the bundled plugin command table")
    return names


def parse_reserved(text: str) -> list[str]:
    """Commands the page says are reserved as built-ins (a user skill of that name gets another alias)."""
    names = re.findall(r"`/([a-z][a-z0-9-]*)`\s+is\s+reserved\s+as\s+a\s+built-in\s+command", text)
    if not names:
        raise ValueError("no reserved built-in command found")
    return names


def extract(ctx) -> dict:
    core = parse_registry(ctx.raw(REGISTRY))
    docs = ctx.raw(DOCS)
    names = core["names"] + [n for n in parse_plugin_commands(docs) if n not in core["names"]]
    skills = []
    for path in ctx.tree(SKILLS):
        if path.endswith("/SKILL.md") and path.count("/") == 2:
            skills.append(fetch.frontmatter_field(ctx.raw(path), "name") or path.split("/")[-2])
    reserved = parse_reserved(docs)
    return {
        "command": {"names": names, "aliases": core["aliases"]},
        "reserved": {"names": reserved},
        "skill": {"names": skills},
    }
