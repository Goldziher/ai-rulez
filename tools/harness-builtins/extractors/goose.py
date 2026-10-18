"""goose: agent slash commands, CLI session commands and bundled skills at the pinned tag."""

import re

from hb import fetch

EXECUTE = "crates/goose/src/agents/execute_commands.rs"
INPUT = "crates/goose-cli/src/session/input.rs"
SKILLS = "crates/goose/src/skills/builtins/"


def strip_comments(text: str) -> str:
    text = re.sub(r"/\*.*?\*/", "", text, flags=re.DOTALL)
    return re.sub(r"(?m)//.*$", "", text)


def parse_agent_commands(text: str) -> list[str]:
    """Names of `static COMMANDS` plus the slash triggers of COMPACT_TRIGGERS (`/summarize`)."""
    text = strip_comments(text)
    m = re.search(r"static COMMANDS[^=]*=\s*&\[(.*?)\n\];", text, flags=re.DOTALL)
    if not m:
        raise ValueError("static COMMANDS not found")
    names = re.findall(r'\bname:\s*"([^"]+)"', m.group(1))
    t = re.search(r"COMPACT_TRIGGERS[^=]*=\s*&\[(.*?)\];", text, flags=re.DOTALL)
    if not t:
        raise ValueError("COMPACT_TRIGGERS not found")
    names += re.findall(r'"/([a-z][a-z0-9-]*)"', t.group(1))
    if not names:
        raise ValueError("no agent commands found")
    return list(dict.fromkeys(names))


def parse_session_commands(text: str) -> dict:
    """The commands `handle_slash_command` of the CLI accepts, with the aliases of `a | b =>` arms."""
    m = re.search(r"fn handle_slash_command\b.*?(?=\nfn |\Z)", strip_comments(text), flags=re.DOTALL)
    if not m:
        raise ValueError("fn handle_slash_command not found")
    body = m.group(0)
    names, aliases = [], {}

    def add(raw: str) -> str:
        name = raw.strip().lstrip("/")
        if name not in names:
            names.append(name)
        return name

    for const in re.findall(r'const CMD_\w+:\s*&str\s*=\s*"(/[^"]+)"', body):
        add(const)
    for arm in re.findall(r'(?m)^\s*("/[^"]*"(?:\s*\|\s*"/[^"]*")*)\s*=>', body):
        alts = re.findall(r'"(/[^"]*)"', arm)
        # a punctuation spelling such as /? is the alias of the word (/help)
        alts.sort(key=lambda a: not a[1:2].isalpha())
        primary = add(alts[0])
        for alt in alts[1:]:
            aliases.setdefault(primary, []).append(alt.lstrip("/"))
    for lit in re.findall(r'starts_with\("(/[^"]+)"\)', body):
        add(lit)
    if not names:
        raise ValueError("no session commands found")
    return {"names": names, "aliases": aliases}


def extract(ctx) -> dict:
    skills = []
    for path in ctx.tree(SKILLS):
        if path.endswith(".md") and "/" not in path[len(SKILLS) :]:
            name = fetch.frontmatter_field(ctx.raw(path), "name")
            if not name:
                raise ValueError(f"{path}: no name in the front matter")
            skills.append(name)
    return {
        "command_agent": {"names": parse_agent_commands(ctx.raw(EXECUTE))},
        "command_session": parse_session_commands(ctx.raw(INPUT)),
        "skill": {"names": skills},
    }
