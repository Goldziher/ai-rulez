"""Write a harness name table in the layout internal/harnessnames/data uses.

The layout is fixed so that regenerating an unchanged table yields the same bytes:
header keys, then [[source]] tables, then [[group]] tables.
"""

HEADER_KEYS = ("schema", "preset", "harness_version", "verified_on", "completeness", "severity")


def q(s: str) -> str:
    return '"' + s.replace("\\", "\\\\").replace('"', '\\"') + '"'


def names_value(names, width=120):
    """One line when `names = [...]` fits in width columns, else one name per line (taplo's layout)."""
    one = "[" + ", ".join(q(n) for n in names) + "]"
    if len("names = ") + len(one) <= width:
        return one
    return "[\n" + "".join(f"  {q(n)},\n" for n in names) + "]"


def aliases_value(aliases, width=120):
    """An inline table; taplo expands every array in it when the line is too long."""
    items = list(aliases.items())
    one = "{ " + ", ".join(f"{q(k)} = [{', '.join(q(a) for a in v)}]" for k, v in items) + " }"
    if len("aliases = ") + len(one) <= width:
        return one
    parts = [f"{q(k)} = [\n" + "".join(f"  {q(a)},\n" for a in v) + "]" for k, v in items]
    return "{ " + ", ".join(parts) + " }"


def dump(t: dict) -> str:
    out = [
        f"schema = {t['schema']}",
        f"preset = {q(t['preset'])}",
        f"harness_version = {q(t['harness_version'])}",
        f"verified_on = {q(t['verified_on'])}",
        f"completeness = {q(t['completeness'])}",
        f"severity = {q(t['severity'])}",
        "invocation_spaces = ["
        + ", ".join("[" + ", ".join(q(k) for k in s) + "]" for s in t["invocation_spaces"])
        + "]",
        f"namespaced = {'true' if t['namespaced'] else 'false'}",
    ]
    for s in t.get("source", []):
        out += ["", "[[source]]", f"id = {q(s['id'])}", f"url = {q(s['url'])}"]
        if s.get("quote"):
            out.append(f"quote = {q(s['quote'])}")
        out.append(f"verified_on = {q(s['verified_on'])}")
    for g in t.get("group", []):
        out += ["", "[[group]]", f"kind = {q(g['kind'])}", f"source = {q(g['source'])}"]
        if g.get("evidence"):
            out.append(f"evidence = {q(g['evidence'])}")
        out.append(f"user_collision = {q(g['user_collision'])}")
        out.append(f"match = {q(g['match'])}")
        out.append("names = " + names_value(g["names"]))
        if g.get("aliases"):
            out.append("aliases = " + aliases_value(g["aliases"]))
    return "\n".join(out) + "\n"
