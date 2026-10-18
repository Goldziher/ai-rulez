"""Regenerate the built-in names of internal/harnessnames/data/<preset>.toml from the
public registry each harness keeps them in, at the ref pinned in pins.toml.

Usage: python3 -I generate.py MODE [--data DIR] [--pins FILE] [PRESET ...]

  --check   fail (exit 1) when a data file differs from what the pinned source yields
  --write   rewrite the data files from the pinned sources
  --bump    look up a newer tag (or branch head) for each pinned repository and show how the
            names would change; with --apply also move the pin and rewrite the data

Exit status: 0 up to date, 1 drift, 2 a source could not be read or an extractor failed.

An extractor (extractors/<name>.py) defines extract(ctx) and returns {key: {"names": [...],
"aliases": {name: [...]}}}. The pin's [[<preset>.target]] tables say which group each key
feeds; every other field of the data file (source ids, quotes, user_collision, evidence,
invocation_spaces, completeness) is kept as it is. A preset with extractor = "none" has no
registry to read and says why in reason.
"""

import argparse
import datetime
import importlib.util
import re
import subprocess
import sys
from pathlib import Path

import tomllib

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))

from hb import fetch, merge, tomlout  # noqa: E402

DEFAULT_DATA = HERE.parent.parent / "internal" / "harnessnames" / "data"
PIN_KEYS = {
    "extractor",
    "reason",
    "repo",
    "ref",
    "track",
    "tag_pattern",
    "harness_version",
    "target",
}
TARGET_KEYS = {"key", "kind", "url_contains", "collision", "add", "exclude"}


class PinError(Exception):
    pass


def load_pins(path: Path) -> dict:
    pins = tomllib.loads(path.read_text(encoding="utf-8"))
    for preset, pin in pins.items():
        if not isinstance(pin, dict):
            raise PinError(f"{preset}: not a table")
        extra = set(pin) - PIN_KEYS
        if extra:
            raise PinError(f"{preset}: unknown keys {sorted(extra)}")
        name = pin.get("extractor")
        if not name:
            raise PinError(f'{preset}: extractor is required (a name, or "none" with a reason)')
        if name == "none":
            if not pin.get("reason"):
                raise PinError(f'{preset}: extractor = "none" needs a reason')
            continue
        if not pin.get("target"):
            raise PinError(f"{preset}: no [[{preset}.target]]")
        if not (HERE / "extractors" / f"{name}.py").is_file():
            raise PinError(f"{preset}: extractors/{name}.py does not exist")
        for t in pin["target"]:
            bad = set(t) - TARGET_KEYS
            if bad or "key" not in t or "kind" not in t:
                raise PinError(f"{preset}: target {t.get('key')!r} needs key and kind; unknown keys {sorted(bad)}")
        if pin.get("repo") and not pin.get("ref"):
            raise PinError(f"{preset}: repo without ref")
    return pins


def load_extractor(name: str):
    spec = importlib.util.spec_from_file_location(f"extractor_{name}", HERE / "extractors" / f"{name}.py")
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def run_extractor(preset: str, pin: dict, get=fetch.http_get) -> dict:
    ctx = fetch.Context(pin.get("repo", ""), pin.get("ref", ""), get)
    return load_extractor(pin["extractor"]).extract(ctx)


def regenerate(table: dict, pin: dict, results: dict) -> dict:
    return merge.merge(table, pin, results)


def check_coverage(pins: dict, data_dir: Path) -> list[str]:
    presets = {p.stem for p in data_dir.glob("*.toml")}
    errs = [
        f'{p}: data file has no pins entry (add one, or extractor = "none" with a reason)'
        for p in sorted(presets - set(pins))
    ]
    errs += [f"{p}: pins entry without a data file" for p in sorted(set(pins) - presets)]
    return errs


def version_key(tag: str):
    return [int(x) for x in re.findall(r"\d+", tag)]


def latest_ref(pin: dict) -> str:
    url = f"https://github.com/{pin['repo']}.git"
    if pin.get("track") == "head":
        out = subprocess.run(["git", "ls-remote", url, "HEAD"], capture_output=True, text=True, check=True).stdout
        return out.split()[0][:12]
    out = subprocess.run(
        ["git", "ls-remote", "--tags", "--refs", url], capture_output=True, text=True, check=True
    ).stdout
    pat = re.compile(pin.get("tag_pattern", r"^v?\d+\.\d+\.\d+$"))
    tags = [line.split("refs/tags/", 1)[1] for line in out.splitlines() if "refs/tags/" in line]
    tags = [t for t in tags if pat.search(t)]
    if not tags:
        raise PinError(f"{pin['repo']}: no tag matches {pat.pattern}")
    return max(tags, key=version_key)


def set_pin_ref(text: str, preset: str, old: str, new: str) -> str:
    """Replace ref = "old" inside the [preset] table of pins.toml text."""
    lines, in_table, done = text.splitlines(keepends=True), False, False
    for i, line in enumerate(lines):
        head = re.match(r"\[\[?([^\]]+)\]\]?\s*$", line)
        if head:
            in_table = head.group(1) == preset
        elif in_table and re.match(r'ref\s*=\s*"' + re.escape(old) + '"', line):
            lines[i] = f'ref = "{new}"\n'
            done = True
    if not done:
        raise PinError(f'{preset}: cannot find ref = "{old}" in pins.toml')
    return "".join(lines)


def process(args, pins, preset, today, out):
    """Handle one preset; returns 1 when --check finds drift, else 0."""
    pin = pins[preset]
    if pin["extractor"] == "none":
        return 0
    path = args.data / f"{preset}.toml"
    old_text = path.read_text(encoding="utf-8")
    old = tomllib.loads(old_text)
    if args.bump:
        if not pin.get("repo"):
            print(f"{preset}: documentation pin, nothing to bump", file=out)
            return 0
        new_ref = latest_ref(pin)
        if new_ref == pin["ref"]:
            print(f"{preset}: {pin['ref']} is the latest", file=out)
            return 0
        bumped = {**pin, "ref": new_ref}
        new = regenerate(old, bumped, run_extractor(preset, bumped))
        print(f"{preset}: {pin['ref']} -> {new_ref}", file=out)
        for line in merge.diff_names(old, new) or ["no change in names"]:
            print(f"  {line}", file=out)
        if args.apply:
            args.pins.write_text(
                set_pin_ref(args.pins.read_text(encoding="utf-8"), preset, pin["ref"], new_ref),
                encoding="utf-8",
                newline="\n",
            )
            merge.stamp(new, bumped, today)
            path.write_text(tomlout.dump(new), encoding="utf-8", newline="\n")
        return 0
    new = regenerate(old, pin, run_extractor(preset, pin))
    stale = tomlout.dump(new) != old_text
    if stale:
        print(f"{preset}: differs from {pin.get('repo', 'the documentation')}@{pin.get('ref', 'live')}", file=out)
        for line in merge.diff_names(old, new):
            print(f"  {line}", file=out)
    else:
        print(f"{preset}: up to date", file=out)
    if args.write:
        merge.stamp(new, pin, today)
        path.write_text(tomlout.dump(new), encoding="utf-8", newline="\n")
    return 1 if stale and args.check else 0


def main(argv=None, out=sys.stdout):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    mode = ap.add_mutually_exclusive_group(required=True)
    mode.add_argument("--check", action="store_true")
    mode.add_argument("--write", action="store_true")
    mode.add_argument("--bump", action="store_true")
    ap.add_argument("--apply", action="store_true", help="with --bump: move the pin and rewrite the data")
    ap.add_argument("--data", type=Path, default=DEFAULT_DATA)
    ap.add_argument("--pins", type=Path, default=HERE / "pins.toml")
    ap.add_argument("--today", default=datetime.datetime.now(datetime.UTC).strftime("%Y-%m-%d"))
    ap.add_argument("presets", nargs="*")
    args = ap.parse_args(argv)
    if args.apply and not args.bump:
        ap.error("--apply needs --bump")
    try:
        pins = load_pins(args.pins)
    except (PinError, tomllib.TOMLDecodeError) as e:
        print(f"pins.toml: {e}", file=sys.stderr)
        return 2
    problems = check_coverage(pins, args.data)
    unknown = [p for p in args.presets if p not in pins]
    if unknown:
        print(f"no pins entry for: {', '.join(unknown)}", file=sys.stderr)
        return 2
    if problems and not args.presets:
        print("\n".join(problems), file=sys.stderr)
        return 2
    drift = errors = 0
    for preset in args.presets or sorted(pins):
        try:
            drift += process(args, pins, preset, args.today, out)
        except (fetch.FetchError, merge.MergeError, PinError, subprocess.CalledProcessError, KeyError, ValueError) as e:
            print(f"{preset}: error: {e}", file=sys.stderr)
            errors += 1
    if errors:
        return 2
    return 1 if drift else 0


if __name__ == "__main__":
    sys.exit(main())
