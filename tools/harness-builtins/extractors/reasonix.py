"""Reasonix: slash commands (the built-in slash registry), shipped skills at the pinned tag."""

import re

from hb import fetch

SLASH = "internal/frontend/cli/slash_registry.go"
BUILTINS = "internal/ext/skill/builtins.go"
EMBEDDED = "internal/ext/skill/builtincontent/"
SPEC = re.compile(r'\{name:\s*"/([^"]+)"(?:,\s*aliases:\s*\[\]string\{([^}]*)\})?')


def strip_comments(text: str) -> str:
    return re.sub(r"(?m)^\s*//.*$", "", text)


def parse_slash(text: str) -> dict[str, list[str]]:
    """{name: [aliases]} of builtinSlashSpecs(), without the leading slash."""
    m = re.search(r"func builtinSlashSpecs\(\)[^{]*\{(.*?)\n\}\n", strip_comments(text), re.DOTALL)
    if not m:
        raise ValueError("builtinSlashSpecs not found")
    out = {
        s.group(1): [a.lstrip("/") for a in re.findall(r'"([^"]+)"', s.group(2) or "")]
        for s in SPEC.finditer(m.group(1))
    }
    if not out:
        raise ValueError("no slash specs found")
    return out


def split_fallback(specs: dict[str, list[str]]) -> tuple[dict, dict]:
    """(commands with a `reasonix:`-qualified fallback alias, all others) as extractor results.

    A command that a custom command or plugin may own gets a qualified alias to stay reachable.
    """
    fallback = {n: a for n, a in specs.items() if any(x.startswith("reasonix:") for x in a)}
    rest = {n: a for n, a in specs.items() if n not in fallback}

    def result(d):
        return {"names": list(d), "aliases": {n: a for n, a in d.items() if a}}

    return result(fallback), result(rest)


def parse_builtin_skills(text: str) -> list[str]:
    """Skill names of builtinSkills(): the `Name:` of each entry (three tabs deep)."""
    m = re.search(r"func builtinSkills\(\)[^{]*\{(.*?)\n\}\n", strip_comments(text), re.DOTALL)
    names = re.findall(r'^\t\t\tName:\s+"([^"]+)",', m.group(1), re.MULTILINE) if m else []
    if not names:
        raise ValueError("no builtin skills found")
    return names


def extract(ctx) -> dict:
    fallback, rest = split_fallback(parse_slash(ctx.raw(SLASH)))
    if not fallback["names"]:
        raise ValueError("no command with a reasonix: fallback alias found")
    skills = parse_builtin_skills(ctx.raw(BUILTINS))
    for path in ctx.tree(EMBEDDED):
        if path.endswith("/SKILL.md") and path.count("/") == EMBEDDED.count("/") + 1:
            skills.append(fetch.frontmatter_field(ctx.raw(path), "name") or path.split("/")[-2])
    return {"fallback": fallback, "command": rest, "skill": {"names": list(dict.fromkeys(skills))}}
