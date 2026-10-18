"""Merge extractor output into a harness name table."""

import copy


class MergeError(Exception):
    pass


def norm(name: str) -> str:
    return name.casefold()


def clean_set(result: dict, target: dict) -> tuple[list[str], dict[str, list[str]]]:
    """Names (sorted, unique) and aliases (sorted by owner) of an extractor result after the
    pin's add and exclude lists."""
    exclude = {norm(n) for n in target.get("exclude", [])}
    names = {n for n in result.get("names", []) if norm(n) not in exclude}
    names |= set(target.get("add", []))
    aliases = {}
    for owner, al in result.get("aliases", {}).items():
        if owner in names:
            keep = sorted({a for a in al if norm(a) not in exclude})
            if keep:
                aliases[owner] = keep
    return sorted(names), dict(sorted(aliases.items()))


def find_group(table: dict, target: dict) -> dict:
    sources = {s["id"]: s for s in table.get("source", [])}
    hits = []
    for g in table.get("group", []):
        if g["kind"] != target["kind"]:
            continue
        if target.get("url_contains", "") not in sources[g["source"]]["url"]:
            continue
        if target.get("collision") and g["user_collision"] != target["collision"]:
            continue
        hits.append(g)
    if len(hits) != 1:
        raise MergeError(
            f"{table['preset']}: target {target['key']!r} (kind={target['kind']}, "
            f"url_contains={target.get('url_contains', '')!r}) matches {len(hits)} groups, want 1"
        )
    return hits[0]


def check_duplicates(table: dict) -> None:
    seen = {}
    for i, g in enumerate(table.get("group", [])):
        for n in g["names"]:
            key = (g["kind"], g["match"], norm(n))
            if key in seen:
                raise MergeError(f"{table['preset']}: {g['kind']} {n!r} is in groups {seen[key] + 1} and {i + 1}")
            seen[key] = i


def apply_ref(table: dict, pin: dict) -> None:
    """Point the table's GitHub blob URLs of the pinned repo at the pinned ref."""
    repo, ref = pin.get("repo"), pin.get("ref")
    if not repo or not ref:
        return
    for kind in ("blob", "tree"):
        prefix = f"https://github.com/{repo}/{kind}/"
        for s in table.get("source", []):
            if not s["url"].startswith(prefix):
                continue
            old, _, path = s["url"][len(prefix) :].partition("/")
            if old != ref:
                # a line anchor of another revision no longer points at the same line
                s["url"] = f"{prefix}{ref}/{path.split('#')[0]}"


def merge(table: dict, pin: dict, results: dict[str, dict]) -> dict:
    """A copy of table with the groups the pin's targets select replaced by the extractor results."""
    out = copy.deepcopy(table)
    apply_ref(out, pin)
    if pin.get("harness_version"):
        out["harness_version"] = pin["harness_version"]
    for target in pin.get("target", []):
        key = target["key"]
        if key not in results:
            raise MergeError(f"{table['preset']}: extractor returned no result for target {key!r}")
        names, aliases = clean_set(results[key], target)
        if not names:
            raise MergeError(f"{table['preset']}: target {key!r} extracted no names")
        g = find_group(out, target)
        g["names"] = names
        if aliases:
            g["aliases"] = aliases
        else:
            g.pop("aliases", None)
    unused = set(results) - {t["key"] for t in pin.get("target", [])}
    if unused:
        raise MergeError(f"{table['preset']}: extractor results without a target: {sorted(unused)}")
    check_duplicates(out)
    return out


def stamp(table: dict, pin: dict, today: str) -> None:
    """Record today as the verification date of the table and of the sources the extractor read."""
    table["verified_on"] = today
    urls = {t.get("url_contains", "") for t in pin.get("target", [])}
    for s in table.get("source", []):
        if any(u in s["url"] for u in urls) and not s.get("quote"):
            s["verified_on"] = today


def diff_names(old: dict, new: dict) -> list[str]:
    """Human-readable differences of names and aliases between two tables."""
    lines = []
    og, ng = old.get("group", []), new.get("group", [])
    for i, (a, b) in enumerate(zip(og, ng, strict=True), 1):
        label = f"group {i} ({a['kind']}/{a['user_collision']})"
        add, rem = sorted(set(b["names"]) - set(a["names"])), sorted(set(a["names"]) - set(b["names"]))
        if add:
            lines.append(f"{label}: + {', '.join(add)}")
        if rem:
            lines.append(f"{label}: - {', '.join(rem)}")
        if not add and not rem and a["names"] != b["names"]:
            lines.append(f"{label}: names reordered")
        if a.get("aliases", {}) != b.get("aliases", {}):
            lines.append(f"{label}: aliases {a.get('aliases', {})} -> {b.get('aliases', {})}")
    for k in ("harness_version",):
        if old[k] != new[k]:
            lines.append(f"{k}: {old[k]!r} -> {new[k]!r}")
    olds = {s["id"]: s["url"] for s in old.get("source", [])}
    for s in new.get("source", []):
        if olds.get(s["id"]) != s["url"]:
            lines.append(f"source {s['id']}: {olds.get(s['id'])} -> {s['url']}")
    return lines
