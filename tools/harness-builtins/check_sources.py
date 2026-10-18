"""Check the quotes of the harness built-in name tables against the live pages.

Usage: python3 -I check_sources.py [--data DIR] [--names] [--stamp] [PRESET ...]

Fetches every source that has a quote (all presets when none is named) and checks that the quote
occurs on the page (whitespace-insensitive). It also fails when a group with a recorded collision
outcome (user_wins, builtin_wins, both_listed, error) has no quote: the outcome would be an
unsourced claim. --names also reports names that do not occur on the page that lists them (not a
failure: a page may spell a name in a form the check does not see). --stamp sets verified_on of
the sources whose quote was found to today.

Exit status: 0 all quotes found, 1 a quote is missing or a recorded outcome has none, 2 a page
could not be fetched.
"""

import argparse
import datetime
import re
import sys
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from hb import fetch, quotes, tomlout  # noqa: E402

DEFAULT_DATA = Path(__file__).resolve().parent.parent.parent / "internal" / "harnessnames" / "data"
OUTCOMES = {"user_wins", "builtin_wins", "both_listed", "error"}
WORKERS = 8


def unquoted_outcomes(table: dict) -> list[str]:
    """Descriptions of the groups that record an outcome without a quote."""
    src = {s["id"]: s for s in table.get("source", [])}
    out = []
    for i, g in enumerate(table.get("group", []), 1):
        if g["user_collision"] not in OUTCOMES:
            continue
        if not src.get(g.get("evidence") or g["source"], {}).get("quote"):
            out.append(f"group {i} ({g['kind']}/{g['user_collision']}, {len(g['names'])} names)")
    return out


def fetch_pages(urls: set[str], get) -> dict:
    """{url: text, or the FetchError}"""

    def one(url: str):
        try:
            return get(quotes.raw_url(url))
        except fetch.FetchError as e:
            return e

    with ThreadPoolExecutor(WORKERS) as pool:
        return dict(zip(sorted(urls), pool.map(one, sorted(urls)), strict=True))


def absent_names(table: dict, pages: dict) -> list[str]:
    out = []
    src = {s["id"]: s for s in table.get("source", [])}
    for g in table.get("group", []):
        page = pages.get(src[g["source"]]["url"])
        if not isinstance(page, str) or g["match"] != "exact":
            continue
        page = quotes.squash(page)
        miss = [
            n for n in g["names"] if not re.search(r"(?<![\w-])" + re.escape(n) + r"(?![\w-])", page, re.IGNORECASE)
        ]
        if miss:
            out.append(
                f"{g['kind']}/{g['user_collision']} {len(g['names']) - len(miss)}/{len(g['names'])} names on {g['source']}; absent: {', '.join(miss[:12])}"
            )
    return out


def check(tables: dict, get, names: bool, out) -> tuple[dict, int]:
    """Returns ({preset: [verified, failed, unfetched]}, exit status)."""
    urls = {s["url"] for t in tables.values() for s in t.get("source", []) if s.get("quote") or names}
    pages = fetch_pages(urls, get)
    stats, status = {}, 0
    for preset, t in tables.items():
        ok = bad = lost = 0
        for s in t.get("source", []):
            if not s.get("quote"):
                continue
            page = pages[s["url"]]
            if isinstance(page, Exception):
                print(f"{preset} {s['id']}: fetch failed: {page}", file=out)
                lost += 1
            elif quotes.has_quote(page, s["quote"]):
                ok += 1
                s["_found"] = True
            else:
                print(f"{preset} {s['id']}: quote MISSING on {s['url']}: {s['quote'][:90]}", file=out)
                bad += 1
        for msg in unquoted_outcomes(t):
            print(f"{preset}: {msg} records an outcome without a quote", file=out)
            bad += 1
        if names:
            for msg in absent_names(t, pages):
                print(f"{preset}: {msg}", file=out)
        stats[preset] = [ok, bad, lost]
        if bad:
            status = 1
        elif lost and status == 0:
            status = 2
    return stats, status


def main(argv=None, out=sys.stdout, get=fetch.http_get) -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--data", type=Path, default=DEFAULT_DATA)
    ap.add_argument("--names", action="store_true")
    ap.add_argument("--stamp", action="store_true")
    ap.add_argument("--today", default=datetime.datetime.now(datetime.UTC).strftime("%Y-%m-%d"))
    ap.add_argument("presets", nargs="*")
    args = ap.parse_args(argv)
    paths = {p.stem: p for p in sorted(args.data.glob("*.toml"))}
    unknown = [p for p in args.presets if p not in paths]
    if unknown:
        print(f"no data file for: {', '.join(unknown)}", file=sys.stderr)
        return 2
    tables = {p: tomlout.load(paths[p]) for p in (args.presets or paths)}
    stats, status = check(tables, get, args.names, out)
    print(f"{'preset':14} {'verified':>8} {'failed':>7} {'unfetched':>9}", file=out)
    for preset, (ok, bad, lost) in stats.items():
        print(f"{preset:14} {ok:>8} {bad:>7} {lost:>9}", file=out)
    total = [sum(v[i] for v in stats.values()) for i in range(3)]
    print(f"{'total':14} {total[0]:>8} {total[1]:>7} {total[2]:>9}", file=out)
    if args.stamp:
        for preset, t in tables.items():
            changed = False
            for s in t.get("source", []):
                if s.pop("_found", False) and s["verified_on"] != args.today:
                    s["verified_on"] = args.today
                    changed = True
            if changed:
                tomlout.save(paths[preset], t)
    return status


if __name__ == "__main__":
    sys.exit(main())
