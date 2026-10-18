"""OpenCode: default slash commands, TUI slash commands, the built-in skill and the native agents.

The parsing helpers are shared with extractors/kilo.py (Kilo is a fork of OpenCode); kilo.py
loads this file by path.
"""

import concurrent.futures
import re

TUI_DIR = "packages/tui/src/"
COMMANDS = "packages/opencode/src/command/index.ts"
SKILLS = "packages/opencode/src/skill/index.ts"
AGENTS = "packages/opencode/src/agent/agent.ts"

STR = r"""(?:"([^"\n]*)"|'([^'\n]*)')"""


def strip_comments(text: str) -> str:
    return re.sub(r"(?m)^[ \t]*//.*$", "", text)


def strings(body: str) -> list[str]:
    return [a or b for a, b in re.findall(STR, body)]


def constant(text: str, ident: str) -> str:
    m = re.search(r"\bconst\s+" + re.escape(ident) + r"\s*=\s*" + STR, text)
    if not m:
        raise ValueError(f"constant {ident} not found")
    return m.group(1) if m.group(1) is not None else m.group(2)


def parse_defaults(text: str) -> list[str]:
    """Names of the commands the command service registers itself: `commands[Default.X] = ...`."""
    text = strip_comments(text)
    m = re.search(r"\bDefault\s*=\s*\{([^}]*)\}", text)
    if not m:
        raise ValueError("Default command table not found")
    table = {}
    for k, a, b in re.findall(r"(\w+)\s*:\s*" + STR, m.group(1)):
        table[k] = a or b
    keys = re.findall(r"commands\[Default\.(\w+)\]\s*=", text)
    if not keys:
        raise ValueError("no default commands registered")
    try:
        return [table[k] for k in keys]
    except KeyError as e:
        raise ValueError(f"Default.{e.args[0]} is not in the Default table") from e


def parse_slashes(text: str) -> list[tuple[str, list[str]]]:
    """(name, aliases) of every TUI command that has a slash name: `slashName: "x"` with an optional
    `slashAliases: [..]`, or `slash: { name: "x", aliases: [..] }`."""
    text = strip_comments(text)
    found = []
    pats = (
        r"\bslashName:\s*" + STR + r"(?=\s*[,}])(?:\s*,\s*slashAliases:\s*\[([^\]]*)\])?",
        r"\bslash:\s*\{\s*name:\s*" + STR + r"(?:\s*,\s*aliases:\s*\[([^\]]*)\])?",
    )
    for pat in pats:
        for m in re.finditer(pat, text):
            found.append((m.group(1) or m.group(2), strings(m.group(3) or "")))
    return found


def parse_builtin_skills(text: str) -> list[str]:
    """Skills the skill service registers before disk discovery: `s.skills[NAME] = {`."""
    text_nc = strip_comments(text)
    names = []
    for m in re.finditer(r"\bskills\[(\w+)\]\s*=\s*\{", text_nc):
        names.append(constant(text_nc, m.group(1)))
    if not names:
        raise ValueError("no built-in skill found")
    return names


def parse_native_agents(text: str) -> list[str]:
    """Names of the entries of the agents record that exist before the user's agent config is applied."""
    text = strip_comments(text)
    start = re.search(r"\bconst agents:\s*Record<string,\s*Info>\s*=\s*\{", text)
    end = text.find("for (const [key, value] of Object.entries(", start.end()) if start else -1
    if end < 0:
        raise ValueError("agents record not found")
    names = re.findall(r"(?m)^\s*\w+:\s*\{\s*\n\s*name:\s*" + STR, text[start.end() : end])
    if not names:
        raise ValueError("no native agents found")
    return [a or b for a, b in names]


def slash_commands(ctx, tui_dir: str) -> dict:
    """The slash commands of every TypeScript file under tui_dir (fetched concurrently)."""
    names, aliases = [], {}
    paths = [p for p in ctx.tree(tui_dir) if p.endswith((".ts", ".tsx")) and ".test." not in p]
    with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
        texts = list(pool.map(ctx.raw, paths))
    for text in texts:
        if "slashName" not in text and "slash:" not in text:
            continue
        for name, al in parse_slashes(text):
            if name not in names:
                names.append(name)
            if al:
                aliases[name] = sorted(set(aliases.get(name, [])) | set(al))
    if not names:
        raise ValueError("no TUI slash commands found")
    return {"names": names, "aliases": aliases}


def extract(ctx) -> dict:
    return {
        "command_default": {"names": parse_defaults(ctx.raw(COMMANDS))},
        "command_tui": slash_commands(ctx, TUI_DIR),
        "skill": {"names": parse_builtin_skills(ctx.raw(SKILLS))},
        "agent": {"names": parse_native_agents(ctx.raw(AGENTS))},
    }
