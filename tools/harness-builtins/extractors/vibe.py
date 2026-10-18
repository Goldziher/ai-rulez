"""Mistral Vibe: slash commands, bundled skills and built-in agent profiles at the pinned tag."""

import ast

COMMANDS = "vibe/cli/commands.py"
SKILLS_INIT = "vibe/core/skills/builtins/__init__.py"
AGENTS = "vibe/core/agents/models.py"


def _parse(text: str, what: str) -> ast.Module:
    try:
        return ast.parse(text)
    except SyntaxError as e:
        raise ValueError(f"{what}: {e}") from e


def _str(node) -> str | None:
    return node.value if isinstance(node, ast.Constant) and isinstance(node.value, str) else None


def parse_commands(text: str) -> dict:
    """{"names", "aliases"} of the dict `_build_commands` returns: keys are the names, the
    `aliases=frozenset([...])` entries other than "/<name>" are the aliases."""
    tree = _parse(text, "commands.py")
    for fn in ast.walk(tree):
        if isinstance(fn, ast.FunctionDef) and fn.name == "_build_commands":
            ret = next((n for n in ast.walk(fn) if isinstance(n, ast.Return) and isinstance(n.value, ast.Dict)), None)
            if ret is None:
                break
            names, aliases = [], {}
            for key, value in zip(ret.value.keys, ret.value.values, strict=True):
                name = _str(key)
                if name is None:
                    raise ValueError("_build_commands: a key is not a string")
                names.append(name)
                found = _aliases(value)
                keep = [a.removeprefix("/") for a in found if a.removeprefix("/") != name]
                if keep:
                    aliases[name] = sorted(set(keep))
            return {"names": names, "aliases": aliases}
    raise ValueError("_build_commands does not return a dict")


def _aliases(call) -> list[str]:
    if not isinstance(call, ast.Call):
        raise ValueError("a command is not a Command(...) call")
    for kw in call.keywords:
        if kw.arg == "aliases" and isinstance(kw.value, ast.Call) and kw.value.args:
            seq = kw.value.args[0]
            if isinstance(seq, ast.List | ast.Tuple | ast.Set):
                return [s for s in map(_str, seq.elts) if s]
    raise ValueError("a command has no aliases=frozenset([...])")


def skill_modules(init_text: str) -> list[str]:
    """Repository paths of the modules `BUILTIN_SKILLS` takes a SKILL from."""
    tree = _parse(init_text, "builtins/__init__.py")
    out = []
    for node in tree.body:
        if isinstance(node, ast.ImportFrom) and node.module and node.module.startswith("vibe.core.skills.builtins."):
            if any(a.name == "SKILL" for a in node.names):
                out.append(node.module.replace(".", "/") + ".py")
    if not out:
        raise ValueError("no builtin skill module is imported")
    return out


def skill_name(text: str) -> str:
    """The name of `SKILL = SkillInfo(name="...")`."""
    for node in _parse(text, "skill module").body:
        if isinstance(node, ast.Assign) and any(isinstance(t, ast.Name) and t.id == "SKILL" for t in node.targets):
            if isinstance(node.value, ast.Call):
                for kw in node.value.keywords:
                    if kw.arg == "name" and _str(kw.value):
                        return _str(kw.value)
    raise ValueError("no SKILL = SkillInfo(name=...) found")


def parse_agents(text: str) -> list[str]:
    """Values of the BuiltinAgentName members that are keys of BUILTIN_AGENTS."""
    tree = _parse(text, "agents/models.py")
    values = {}
    for node in tree.body:
        if isinstance(node, ast.ClassDef) and node.name == "BuiltinAgentName":
            for stmt in node.body:
                if isinstance(stmt, ast.Assign) and _str(stmt.value):
                    values[stmt.targets[0].id] = _str(stmt.value)
    for node in tree.body:
        target = node.target if isinstance(node, ast.AnnAssign) else None
        if isinstance(target, ast.Name) and target.id == "BUILTIN_AGENTS" and isinstance(node.value, ast.Dict):
            names = []
            for key in node.value.keys:
                if not (isinstance(key, ast.Attribute) and key.attr in values):
                    raise ValueError("BUILTIN_AGENTS has a key that is not a BuiltinAgentName member")
                names.append(values[key.attr])
            return names
    raise ValueError("BUILTIN_AGENTS not found")


def extract(ctx) -> dict:
    skills = [skill_name(ctx.raw(path)) for path in skill_modules(ctx.raw(SKILLS_INIT))]
    return {
        "command": parse_commands(ctx.raw(COMMANDS)),
        "skill": {"names": skills},
        "agent": {"names": parse_agents(ctx.raw(AGENTS))},
    }
