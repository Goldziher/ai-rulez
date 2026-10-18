"""Convert the compiled harness built-in name tables (GitHub issue #447) to
internal/harnessnames/data/<preset>.toml.

Usage: python3 -I convert.py OUT_DIR ISSUE_BODY.md COMMENT.md [COMMENT.md ...]

Input is the markdown of the issue body and its comments: a "### `preset` - Title"
section per harness with a "- version: ...; verified_on: ...; completeness: ..."
line and a "| kind | collision | source | names |" table. Entries without a
source are not in the input. Prose fields (precedence, invocation, gaps) are not
carried over; the sources are.

quotes.toml (next to this script) adds the verbatim quotes that were checked
against the live page and extra hand-recorded groups.
"""

import re
import sys
from pathlib import Path

import tomllib

ROLLOUT_ERROR = {
    "claude",
    "codex",
    "gemini",
    "opencode",
    "cline",
    "qwen",
    "goose",
    "kilo",
    "zoocode",
}
# Kinds that share one namespace. Codex has no user slash commands. OpenCode and Kilo skip a skill
# whose name an existing command already has ("if (commands[item.name]) continue"), so a skill
# never replaces a command and the two do not collide.
SPACES = {
    "codex": [["skill"], ["agent"]],
    "opencode": [["command"], ["skill"], ["agent"]],
    "kilo": [["command"], ["skill"], ["agent"]],
}
DEFAULT_SPACES = [["command", "skill"], ["agent"]]
COLLISIONS = {"user_wins", "builtin_wins", "both_listed", "error", "n/a", "unknown"}
HEADER = re.compile(r"^### `([a-z0-9-]+)` - ")
VERSION = re.compile(r"^- version: (.*?); verified_on: (\d{4}-\d{2}-\d{2}); completeness: (\w[\w-]*)\s*$")
NAME = re.compile(r"^(.*?)(?: \(aliases: ([^)]*)\))?$")
UNCONFIRMED = re.compile(r"\((?:name )?unconfirmed\)")


def q(s: str) -> str:
    return '"' + s.replace("\\", "\\\\").replace('"', '\\"') + '"'


def wrap(items, indent="  ", width=100):
    lines, cur = [], indent
    for it in items:
        piece = q(it) + ", "
        if len(cur) + len(piece) > width and cur.strip():
            lines.append(cur.rstrip())
            cur = indent
        cur += piece
    if cur.strip():
        lines.append(cur.rstrip())
    return "[\n" + "\n".join(lines) + "\n]"


def parse(files):
    presets, cur = {}, None
    for f in files:
        for line in Path(f).read_text(encoding="utf-8").splitlines():
            m = HEADER.match(line)
            if m:
                cur = presets.setdefault(m.group(1), {"rows": []})
                continue
            if cur is None:
                continue
            m = VERSION.match(line)
            if m:
                cur["version"], cur["verified_on"], cur["completeness"] = m.groups()
                continue
            if line.startswith("| ") and "---" not in line and not line.startswith("| kind"):
                cells = [c.strip() for c in line.strip().strip("|").split(" | ")]
                if len(cells) == 4:
                    cur["rows"].append(cells)
    return presets


def kind_of(cell):
    base = cell.split(" ")[0]
    return base, "reserved" in cell


def build(preset, data, quotes, dropped):
    srcs, groups, seen = {}, [], set()
    extra = quotes.get(preset, {})
    replace = set(extra.get("replace", []))
    quoted = {}  # source id -> quote

    def sid_for(url, quote=None):
        # one source per (url, quote): a page can document several claims
        sid = srcs.setdefault((url, quote), f"s{len(srcs) + 1}")
        if quote:
            quoted[sid] = quote
        return sid

    for kind_cell, coll, src, names_cell in data["rows"]:
        kind, reserved = kind_of(kind_cell)
        kind = "reserved" if reserved else kind
        if kind in replace:
            continue
        url = src.strip("<>")
        if not url.startswith("https://"):
            dropped.append((preset, kind_cell, "no source url"))
            continue
        coll = extra.get("collision_override", {}).get(kind_cell, coll)
        if coll not in COLLISIONS:
            raise SystemExit(f"{preset}: unknown collision {coll!r}")
        sid = sid_for(url)
        names, aliases = [], {}
        for raw in re.findall(r"`([^`]*)`", names_cell):
            if UNCONFIRMED.search(raw):
                dropped.append((preset, raw, "name unconfirmed"))
                continue
            name, al = NAME.match(raw).groups()
            key = (kind, name.casefold())
            if key in seen:
                dropped.append((preset, raw, "duplicate"))
                continue
            seen.add(key)
            names.append(name)
            if al:
                aliases[name] = [a.strip() for a in al.split(",") if a.strip()]
        if names:
            groups.append(
                {
                    "kind": kind,
                    "source": sid,
                    "collision": coll,
                    "match": "exact",
                    "names": names,
                    "aliases": aliases,
                    "evidence": "",
                }
            )
    for g in extra.get("group", []):
        groups.append(
            {
                "kind": g["kind"],
                "source": sid_for(g["url"], g.get("quote")),
                "collision": g["collision"],
                "match": g.get("match", "exact"),
                "names": g["names"],
                "aliases": {},
                "evidence": "",
            }
        )
    for ev in extra.get("evidence", []):
        sid = sid_for(ev["url"], ev["quote"])
        for g in groups:
            if g["evidence"] or g["collision"] in ("unknown", "n/a"):
                continue
            if ev.get("kinds") and g["kind"] not in ev["kinds"]:
                continue
            if ev.get("collisions") and g["collision"] not in ev["collisions"]:
                continue
            g["evidence"] = sid
    for g in groups:
        if g["evidence"] == "" and g["source"] in quoted and g["collision"] not in ("unknown", "n/a"):
            g["evidence"] = g["source"]
    out = [
        "schema = 1",
        f"preset = {q(preset)}",
        f"harness_version = {q(data['version'])}",
        f"verified_on = {q(data['verified_on'])}",
        f"completeness = {q(data['completeness'])}",
        f"severity = {q('error' if preset in ROLLOUT_ERROR else 'warning')}",
    ]
    spaces = SPACES.get(preset, DEFAULT_SPACES)
    out.append("invocation_spaces = [" + ", ".join("[" + ", ".join(q(k) for k in s) + "]" for s in spaces) + "]")
    out.append(f"namespaced = {'true' if preset == 'claude' else 'false'}")
    for (url, _), sid in srcs.items():
        out += ["", "[[source]]", f"id = {q(sid)}", f"url = {q(url)}"]
        if sid in quoted:
            out.append(f"quote = {q(quoted[sid])}")
        out.append(f"verified_on = {q(data['verified_on'])}")
    for g in groups:
        out += ["", "[[group]]", f"kind = {q(g['kind'])}", f"source = {q(g['source'])}"]
        if g["evidence"] and g["evidence"] != g["source"]:
            out.append(f"evidence = {q(g['evidence'])}")
        out.append(f"user_collision = {q(g['collision'])}")
        out.append(f"match = {q(g['match'])}")
        out.append("names = " + wrap(g["names"]))
        if g["aliases"]:
            out.append(
                "aliases = { "
                + ", ".join(f"{q(k)} = [{', '.join(q(a) for a in v)}]" for k, v in g["aliases"].items())
                + " }"
            )
    return "\n".join(out) + "\n"


def main():
    out_dir, files = Path(sys.argv[1]), sys.argv[2:]
    qfile = Path(__file__).with_name("quotes.toml")
    quotes = tomllib.loads(qfile.read_text(encoding="utf-8")) if qfile.exists() else {}
    presets = parse(files)
    dropped = []
    out_dir.mkdir(parents=True, exist_ok=True)
    for preset, data in sorted(presets.items()):
        (out_dir / f"{preset}.toml").write_text(build(preset, data, quotes, dropped), encoding="utf-8", newline="\n")
    print(f"{len(presets)} presets, {len(dropped)} entries dropped")
    for d in dropped:
        print("  dropped:", *d)


if __name__ == "__main__":
    main()
