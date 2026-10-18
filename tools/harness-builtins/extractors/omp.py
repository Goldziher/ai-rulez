"""Oh My Pi (omp): built-in slash commands and bundled agents at the pinned tag."""

import re

from hb import fetch

SLASH_DIR = "packages/coding-agent/src/slash-commands/"
# the registry (builtin-registry.ts) spreads the BUILTIN_*_SLASH_COMMANDS arrays of these files; the
# registry and completion helpers hold no commands
NOT_COMMANDS = ("builtin-registry.ts", "builtin-completions.ts")
AGENTS = "packages/coding-agent/src/task/agents.ts"
AGENT_PROMPTS = "packages/coding-agent/src/prompts/agents/"


def parse_commands(text: str) -> dict[str, list[str]]:
    """{name: [aliases]} of the top-level spec objects of one builtin-*.ts file.

    Specs sit one level inside the exported array (two tabs for their fields); sub-command definitions
    are nested deeper and are not matched.
    """
    text = re.sub(r"(?m)^\s*//.*$", "", text)
    out, current = {}, None
    for line in text.splitlines():
        m = re.match(r'\t\tname: "([^"]+)",\s*$', line)
        if m:
            current = m.group(1)
            out[current] = []
            continue
        m = re.match(r"\t\taliases: \[([^\]]*)\],\s*$", line)
        if m and current:
            out[current] = re.findall(r'"([^"]+)"', m.group(1))
    return out


def parse_embedded_agents(text: str) -> list[tuple[str, str | None]]:
    """(file name, name set inline) of the entries of EMBEDDED_AGENT_DEFS."""
    m = re.search(r"const EMBEDDED_AGENT_DEFS\b[^=]*=\s*\[(.*?)\n\];", text, re.DOTALL)
    if not m:
        raise ValueError("EMBEDDED_AGENT_DEFS not found")
    body = re.sub(r"(?m)^\s*//.*$", "", m.group(1))
    parts = re.split(r'(?=\bfileName:\s*")', body)[1:]
    out = []
    for part in parts:
        file = re.match(r'fileName:\s*"([^"]+)"', part).group(1)
        inline = re.search(r'\bname:\s*"([^"]+)"', part)
        out.append((file, inline.group(1) if inline else None))
    if not out:
        raise ValueError("no embedded agents found")
    return out


def extract(ctx) -> dict:
    names, aliases = [], {}
    for path in ctx.tree(SLASH_DIR):
        base = path[len(SLASH_DIR) :]
        if "/" in base or not base.startswith("builtin-") or base in NOT_COMMANDS or not base.endswith(".ts"):
            continue
        for name, al in parse_commands(ctx.raw(path)).items():
            names.append(name)
            if al:
                aliases[name] = al
    agents = []
    for file, inline in parse_embedded_agents(ctx.raw(AGENTS)):
        agents.append(inline or fetch.frontmatter_field(ctx.raw(AGENT_PROMPTS + file), "name") or file[:-3])
    return {"command": {"names": names, "aliases": aliases}, "agent": {"names": agents}}
