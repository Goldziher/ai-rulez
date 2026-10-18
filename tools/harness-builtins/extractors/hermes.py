"""Hermes Agent: the slash command registry and the bundled skills at the pinned commit."""

import ast
import re

from hb import fetch

COMMANDS = "hermes_cli/commands.py"
SKILLS = "skills/"
SKIP_PARTS = {".git", ".github", ".hub", ".archive", ".locks"}  # _SCAN_SKIP_PARTS of agent/skill_commands.py


def parse_registry(text: str) -> dict:
    """{"names", "aliases"} of the CommandDef(...) entries of COMMAND_REGISTRY."""
    try:
        tree = ast.parse(text)
    except SyntaxError as e:
        raise ValueError(f"commands.py: {e}") from e
    for node in tree.body:
        target = node.target if isinstance(node, ast.AnnAssign) else None
        if isinstance(target, ast.Name) and target.id == "COMMAND_REGISTRY" and isinstance(node.value, ast.List):
            names, aliases = [], {}
            for call in node.value.elts:
                if not (isinstance(call, ast.Call) and getattr(call.func, "id", "") == "CommandDef" and call.args):
                    raise ValueError("COMMAND_REGISTRY has an entry that is not CommandDef(...)")
                first = call.args[0]
                if not (isinstance(first, ast.Constant) and isinstance(first.value, str)):
                    raise ValueError("a CommandDef name is not a string literal")
                names.append(first.value)
                for kw in call.keywords:
                    if kw.arg == "aliases":
                        found = [e.value for e in getattr(kw.value, "elts", []) if isinstance(e, ast.Constant)]
                        if len(found) != len(getattr(kw.value, "elts", [])):
                            raise ValueError(f"{first.value}: aliases are not string literals")
                        aliases[first.value] = found
            if not names:
                raise ValueError("COMMAND_REGISTRY is empty")
            return {"names": names, "aliases": aliases}
    raise ValueError("COMMAND_REGISTRY not found")


def slugify(name: str) -> str:
    """slugify_skill_name of agent/skill_commands.py: the /command a skill name registers."""
    cmd = re.sub(r"[^\w-]", "", name.lower().replace(" ", "-").replace("_", "-"))
    return re.sub(r"-{2,}", "-", cmd).strip("-")


def skill_paths(paths: list[str]) -> list[str]:
    """SKILL.md files of the bundled tree that Hermes scans."""
    out = [p for p in paths if p.endswith("/SKILL.md") and not SKIP_PARTS.intersection(p.split("/"))]
    if not out:
        raise ValueError("no bundled SKILL.md found")
    return out


def skill_command(text: str, path: str) -> str:
    name = fetch.frontmatter_field(text, "name") or path.split("/")[-2]
    slug = slugify(name)
    if not slug:
        raise ValueError(f"{path}: {name!r} has no command slug")
    return slug


def extract(ctx) -> dict:
    skills = [skill_command(ctx.raw(p), p) for p in skill_paths(ctx.tree(SKILLS))]
    return {"command": parse_registry(ctx.raw(COMMANDS)), "skill": {"names": skills}}
