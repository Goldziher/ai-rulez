"""Gemini CLI: slash commands, bundled skills and built-in agents at the pinned tag.

The parsing helpers here are also used by extractors/qwen.py (Qwen Code is a fork with the
same BuiltinCommandLoader layout); qwen.py loads this file by path.
"""

import posixpath
import re

from hb import fetch

LOADER = "packages/cli/src/services/BuiltinCommandLoader.ts"
SKILLS = "packages/core/src/skills/builtin/"
REGISTRY = "packages/core/src/agents/registry.ts"
AGENTS_DIR = "packages/core/src/agents/"

IMPORT_RE = re.compile(r"import\s*\{([^}]*)\}\s*from\s*'([^']+)'")
STRING = r"""(?:'([^']*)'|"([^"]*)")"""


def strip_comments(text: str) -> str:
    text = re.sub(r"(?m)^[ \t]*/\*.*?\*/", "", text, flags=re.DOTALL)
    return re.sub(r"(?m)^\s*//.*$", "", text)


def parse_imports(text: str) -> dict[str, str]:
    """Identifier -> module path of the named imports (`a as b` imports bind b)."""
    out = {}
    for m in IMPORT_RE.finditer(text):
        for part in m.group(1).split(","):
            part = re.sub(r"^\s*type\s+", "", part.strip())
            if part:
                out[part.split(" as ")[-1].strip()] = m.group(2)
    return out


def array_body(text: str, marker: str) -> str:
    """The text between the [ that follows marker and its matching ]."""
    m = re.search(re.escape(marker) + r"[^=]*=\s*\[", text)
    if not m:
        raise ValueError(f"{marker} array not found")
    depth, i = 1, m.end()
    while depth and i < len(text):
        depth += {"[": 1, "]": -1}.get(text[i], 0)
        i += 1
    if depth:
        raise ValueError(f"{marker} array is not closed")
    return text[m.end() : i - 1]


def listed_identifiers(body: str, imports: dict[str, str]) -> list[str]:
    """Imported identifiers named in body, in order, once each (conditional entries count)."""
    seen = []
    for tok in re.findall(r"[A-Za-z_$][\w$]*", body):
        if tok in imports and tok not in seen:
            seen.append(tok)
    return seen


def resolve_constant(text: str, ident: str) -> str:
    m = re.search(r"\bconst\s+" + re.escape(ident) + r"\s*=\s*" + STRING, text)
    if not m:
        raise ValueError(f"constant {ident} not found")
    return m.group(1) if m.group(1) is not None else m.group(2)


def declared_name(text: str, ident: str) -> tuple[str, list[str]]:
    """The name (and altNames) of the object the exported declaration `ident` builds: the first
    `name:` after the declaration, with the altNames that directly follow it."""
    text = strip_comments(text)
    decl = re.search(r"\bexport\s+(?:async\s+)?(?:const|function)\s+" + re.escape(ident) + r"\b", text)
    if not decl:
        raise ValueError(f"declaration of {ident} not found")
    m = re.compile(r"\bname:\s*(?:" + STRING + r"|([A-Za-z_$][\w$]*))").search(text, decl.end())
    if not m:
        raise ValueError(f"{ident}: no name field")
    if m.group(3):
        name = resolve_constant(text, m.group(3))
    else:
        name = m.group(1) if m.group(1) is not None else m.group(2)
    alt = re.compile(r"\s*,\s*altNames:\s*\[([^\]]*)\]").match(text, m.end())
    aliases = re.findall(STRING, alt.group(1)) if alt else []
    return name, [a or b for a, b in aliases]


def read_module(ctx, from_file: str, module: str) -> str:
    """The text of the TypeScript module an import path names, relative to the importing file."""
    base = posixpath.normpath(posixpath.join(posixpath.dirname(from_file), re.sub(r"\.js$", "", module)))
    err = None
    for ext in (".ts", ".tsx"):
        try:
            return ctx.raw(base + ext)
        except fetch.FetchError as e:
            err = e
    raise err


def commands(ctx, loader: str, extra: tuple[str, ...] = ()) -> dict:
    text = ctx.raw(loader)
    imports = parse_imports(text)
    body = array_body(strip_comments(text), "allDefinitions")
    idents = listed_identifiers(body, imports)
    idents += [i for i in extra if i in imports and i not in idents]
    names, aliases = [], {}
    for ident in idents:
        if not ident.endswith("Command") or not imports[ident].startswith("."):
            continue  # CommandKind, CommandContext and other helpers the inline entries use
        name, alt = declared_name(read_module(ctx, loader, imports[ident]), ident)
        names.append(name)
        if alt:
            aliases[name] = alt
    if not names:
        raise ValueError("no built-in commands found")
    return {"names": names, "aliases": aliases}


def skills(ctx, prefix: str) -> dict:
    names = []
    for path in ctx.tree(prefix):
        if path.endswith("/SKILL.md") and path.count("/") == prefix.count("/") + 1:
            names.append(fetch.frontmatter_field(ctx.raw(path), "name") or path.split("/")[-2])
    if not names:
        raise ValueError("no bundled skills found")
    return {"names": names}


def builtin_agent_calls(text: str) -> list[str]:
    """Identifiers of the factories loadBuiltInAgents registers (conditional ones included)."""
    m = re.search(r"\bloadBuiltInAgents\s*\(\s*\)\s*(?::\s*\w+\s*)?\{", text)
    if not m:
        raise ValueError("loadBuiltInAgents not found")
    depth, i = 1, m.end()
    while depth and i < len(text):
        depth += {"{": 1, "}": -1}.get(text[i], 0)
        i += 1
    body = strip_comments(text[m.end() : i - 1])
    found = re.findall(r"registerLocalAgent\(\s*(\w+)\(", body)
    if not found:
        raise ValueError("no built-in agents registered")
    return found


def agents(ctx, registry: str) -> dict:
    text = ctx.raw(registry)
    imports = parse_imports(text)
    names = []
    for ident in builtin_agent_calls(text):
        if ident not in imports:
            raise ValueError(f"agent factory {ident} is not imported")
        names.append(declared_name(read_module(ctx, registry, imports[ident]), ident)[0])
    return {"names": names}


def extract(ctx) -> dict:
    return {
        "command": commands(ctx, LOADER),
        "skill": skills(ctx, SKILLS),
        "agent": agents(ctx, REGISTRY),
    }
