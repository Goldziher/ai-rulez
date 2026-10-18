"""Fetch the sources of data files and report names and quotes the page does not contain.

Usage: python3 -I check_sources.py DATA_DIR PRESET [PRESET ...]

A GitHub blob URL is read from raw.githubusercontent.com. A name counts as present
when it occurs in the page as a whole word (case-insensitive). Exit status 1 when a
quote is missing. A missing name is reported but is not a failure: a page may list a
name in a form the check does not see (a table cell, an enum variant).
"""

import re
import subprocess
import sys
from pathlib import Path

import tomllib


def raw(url):
    m = re.match(r"https://github\.com/([^/]+/[^/]+)/blob/([^/]+)/(.*?)(#.*)?$", url)
    if m:
        return f"https://raw.githubusercontent.com/{m.group(1)}/{m.group(2)}/{m.group(3)}"
    return url.split("#")[0]


def fetch(url):
    # curl uses the system trust store; Python's bundled one is not always set up
    out = subprocess.run(
        ["curl", "-sSLf", "-m", "60", "-A", "ai-rulez-harness-check", raw(url)], capture_output=True, check=True
    )
    return out.stdout.decode("utf-8", "replace")


def squash(s):
    return re.sub(r"\s+", " ", s)


def main():
    data_dir, presets = Path(sys.argv[1]), sys.argv[2:]
    bad = 0
    for preset in presets:
        t = tomllib.loads((data_dir / f"{preset}.toml").read_text())
        pages = {}
        for s in t.get("source", []):
            try:
                pages[s["id"]] = squash(fetch(s["url"]))
            except subprocess.CalledProcessError as e:
                print(f"{preset} {s['url']}: fetch failed (curl exit {e.returncode})")
                bad += 1
                continue
            if s.get("quote"):
                ok = squash(s["quote"]) in pages[s["id"]]
                print(f"{preset} {s['id']} quote {'OK' if ok else 'MISSING'}: {s['quote'][:70]}")
                bad += 0 if ok else 1
        for g in t.get("group", []):
            page = pages.get(g["source"])
            if page is None or g["match"] != "exact":
                continue
            missing = [
                n for n in g["names"] if not re.search(r"(?<![\w-])" + re.escape(n) + r"(?![\w-])", page, re.IGNORECASE)
            ]
            print(
                f"{preset} {g['kind']}/{g['user_collision']} {len(g['names']) - len(missing)}/{len(g['names'])} names on {g['source']}"
                + (f"; absent: {', '.join(missing[:12])}" if missing else "")
            )
    sys.exit(1 if bad else 0)


if __name__ == "__main__":
    main()
