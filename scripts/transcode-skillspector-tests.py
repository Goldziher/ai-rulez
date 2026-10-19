"""Transcode SkillSpector pattern tests into candidate Go ruleCase rows.

Reads Python test files that call an analyzer with (content, filename[, type]) and assert that a
rule id is present or absent in the findings, and prints one ruleCase row per scenario.

    transcode-skillspector-tests.py --tests DIR --rule 'DS[1-4]' --map DS1=AR044 --upstream SHA
"""

from __future__ import annotations

import argparse
import ast
import itertools
import re
import subprocess
import sys
import textwrap
import unicodedata
from collections import Counter
from dataclasses import dataclass, field
from pathlib import Path, PurePosixPath

RULE_ID = re.compile(r"^[A-Z]{1,5}\d{1,2}$")
CALLEES = {"analyze", "_run", "_scan", "_analyze", "_findings", "scan"}
MAX_EXPANSION = 64
STATE_CALLEES = {"run_static_patterns", "node"}
DEFAULT_FILE = "SKILL.md"
FILENAME = re.compile(r"\.\w+$|^[A-Z_]+$")


class Unresolved(Exception):
    """Raised when an expression cannot be evaluated statically."""


@dataclass
class Scenario:
    test: str
    source: str
    files: dict[str, str | bytes]
    want: set[str] = field(default_factory=set)
    absent: set[str] = field(default_factory=set)
    absent_all: bool = False
    var: str = ""
    aliases: dict[str, set[str]] = field(default_factory=dict)

    @property
    def filename(self) -> str:
        return next(iter(self.files))


def evaluate(node: ast.AST, env: dict[str, object]) -> object:
    try:
        return evaluate_node(node, env)
    except (
        TypeError,
        ValueError,
        IndexError,
        KeyError,
        AttributeError,
        OverflowError,
        MemoryError,
    ) as exc:
        raise Unresolved(type(exc).__name__) from exc


def evaluate_node(node: ast.AST, env: dict[str, object]) -> object:
    match node:
        case ast.Constant():
            return node.value
        case ast.Name():
            if node.id in env:
                return env[node.id]
            raise Unresolved(node.id)
        case ast.JoinedStr():
            return "".join(str(evaluate(v, env)) for v in node.values)
        case ast.FormattedValue():
            return evaluate(node.value, env)
        case ast.BinOp():
            return binop(node, env)
        case ast.Attribute() if node.attr == "parent":
            return PurePosixPath(str(evaluate(node.value, env))).parent
        case ast.Dict():
            return {evaluate(k, env): evaluate(v, env) for k, v in zip(node.keys, node.values, strict=True) if k}
        case ast.Tuple() | ast.List() | ast.Set():
            return [evaluate(e, env) for e in node.elts]
        case ast.Call():
            return call(node, env)
        case ast.GeneratorExp() | ast.ListComp():
            return comprehension(node, env)
    raise Unresolved(ast.dump(node)[:60])


def binop(node: ast.BinOp, env: dict[str, object]) -> object:
    left, right = evaluate(node.left, env), evaluate(node.right, env)
    match node.op:
        case ast.Div():
            return PurePosixPath(str(left)) / str(right)
        case ast.Add():
            return left + right  # type: ignore[operator]
        case ast.Mult():
            return left * right  # type: ignore[operator]
        case ast.Mod():
            return left % tuple(right) if isinstance(right, list) else left % right  # type: ignore[operator]
    raise Unresolved("binop")


def comprehension(node: ast.GeneratorExp | ast.ListComp, env: dict[str, object]) -> list[object]:
    if len(node.generators) != 1 or node.generators[0].ifs or not isinstance(node.generators[0].target, ast.Name):
        raise Unresolved("comprehension")
    gen = node.generators[0]
    items = evaluate(gen.iter, env)
    if isinstance(items, str):
        items = list(items)
    if not isinstance(items, list | range):
        raise Unresolved("iter")
    return [evaluate(node.elt, {**env, gen.target.id: item}) for item in items]  # type: ignore[union-attr]


def call(node: ast.Call, env: dict[str, object]) -> object:
    name = dotted(node.func)
    args = [evaluate(a, env) for a in node.args]
    if name.endswith(".param"):
        return args
    if name == "chr":
        return chr(args[0])  # type: ignore[arg-type]
    if name == "ord":
        return ord(args[0])  # type: ignore[arg-type]
    if name == "range":
        return range(*args)  # type: ignore[arg-type]
    if name in {"int", "len"} or name == "str" and not isinstance(args[0], PurePosixPath):
        return {"str": str, "int": int, "len": len}[name](args[0])  # type: ignore[operator]
    if name == "str" and args and isinstance(args[0], PurePosixPath):
        return str(args[0])
    if name.endswith("dedent"):
        return textwrap.dedent(args[0])  # type: ignore[arg-type]
    if isinstance(node.func, ast.Attribute):
        return method(node, args, env)
    raise Unresolved(name)


def method(node: ast.Call, args: list[object], env: dict[str, object]) -> object:
    func = node.func
    assert isinstance(func, ast.Attribute)
    target = evaluate(func.value, env)
    if isinstance(target, str) and func.attr in {
        "join",
        "format",
        "replace",
        "strip",
        "lower",
        "upper",
        "encode",
    }:
        if func.attr == "join":
            return target.join(str(x) for x in args[0])  # type: ignore[attr-defined]
        if func.attr == "format":
            kwargs = {k.arg: evaluate(k.value, env) for k in node.keywords if k.arg}
            return target.format(*args, **kwargs)
        if func.attr == "encode":
            raise Unresolved("bytes")
        return getattr(target, func.attr)(*args)
    raise Unresolved(func.attr)


def dotted(node: ast.AST) -> str:
    if isinstance(node, ast.Attribute):
        return f"{dotted(node.value)}.{node.attr}"
    if isinstance(node, ast.Name):
        return node.id
    return ""


def module_env(tree: ast.Module) -> dict[str, object]:
    env: dict[str, object] = {}
    for stmt in tree.body:
        if isinstance(stmt, ast.Assign) and len(stmt.targets) == 1 and isinstance(stmt.targets[0], ast.Name):
            try:
                env[stmt.targets[0].id] = evaluate(stmt.value, env)
            except Unresolved:
                continue
    return env


def parametrizations(func: ast.FunctionDef, env: dict[str, object]) -> list[dict[str, object]]:
    rows: list[dict[str, object]] = [{}]
    for dec in func.decorator_list:
        if not (isinstance(dec, ast.Call) and dotted(dec.func).endswith("parametrize") and len(dec.args) >= 2):
            continue
        try:
            spec = evaluate(dec.args[0], env)
            names = [n.strip() for n in (spec if isinstance(spec, list) else str(spec).split(","))]
            values = evaluate(dec.args[1], env)
        except Unresolved:
            return []
        entries = [v if len(names) > 1 else [v] for v in values]  # type: ignore[union-attr]
        rows = [{**r, **dict(zip(names, e, strict=False))} for r, e in itertools.product(rows, entries)]
        if len(rows) > MAX_EXPANSION:
            return []
    return rows


def walk_statements(body: list[ast.stmt]):
    for stmt in body:
        yield stmt
        for attr in ("body", "orelse", "finalbody"):
            inner = getattr(stmt, attr, None)
            if isinstance(inner, list) and not isinstance(stmt, ast.FunctionDef | ast.ClassDef):
                yield from walk_statements(inner)


def find_analyze_call(node: ast.AST) -> ast.Call | None:
    for sub in ast.walk(node):
        if isinstance(sub, ast.Call) and sub.args and callee(sub) in CALLEES | STATE_CALLEES:
            return sub
    return None


def callee(node: ast.Call) -> str:
    return dotted(node.func).split(".")[-1]


def input_files(hit: ast.Call, env: dict[str, object]) -> dict[str, str] | None:
    values = []
    for arg in hit.args:
        try:
            values.append(evaluate(arg, env))
        except Unresolved:
            values.append(None)
    for value in values:
        if isinstance(value, dict) and isinstance(value.get("file_cache"), dict):
            cache = value["file_cache"]
            if cache and all(isinstance(k, str) and isinstance(v, str) for k, v in cache.items()):
                return dict(cache)
    if callee(hit) in STATE_CALLEES or not isinstance(values[0], str):
        return None
    if len(values) > 1 and isinstance(values[1], str) and FILENAME.search(values[1]):
        return {values[1]: values[0]}
    return {DEFAULT_FILE: values[0]} if len(values) == 1 else None


def rule_ids(node: ast.AST, env: dict[str, object]) -> list[str]:
    ids = []
    for sub in ast.walk(node):
        if isinstance(sub, ast.Compare) and "rule_id" in ast.dump(sub.left) and len(sub.comparators) == 1:
            ids.extend(strings(sub.comparators[0], env, RULE_ID))
    return ids


def strings(node: ast.AST, env: dict[str, object], pattern: re.Pattern[str]) -> list[str]:
    try:
        value = evaluate(node, env)
    except Unresolved:
        return []
    values = value if isinstance(value, list) else [value]
    return [v for v in values if isinstance(v, str) and pattern.match(v)]


def startswith_prefixes(node: ast.AST) -> list[str]:
    return [
        a.value
        for sub in ast.walk(node)
        if isinstance(sub, ast.Call) and dotted(sub.func).endswith("rule_id.startswith")
        for a in sub.args
        if isinstance(a, ast.Constant) and isinstance(a.value, str) and len(a.value) > 1
    ]


def is_zero(node: ast.AST) -> bool:
    return isinstance(node, ast.Constant) and node.value == 0 or isinstance(node, ast.List) and not node.elts


def polarity_flips(test: ast.expr) -> bool:
    if isinstance(test, ast.UnaryOp) and isinstance(test.op, ast.Not):
        return True
    if isinstance(test, ast.Compare) and len(test.comparators) == 1:
        return isinstance(test.ops[0], ast.Eq | ast.Is) and is_zero(test.comparators[0])
    return False


def collect(func: ast.FunctionDef, bind: dict[str, object], base: dict[str, object], source: str) -> list[Scenario]:
    env = {**base, **bind}
    scenarios: list[Scenario] = []
    current: Scenario | None = None
    written: dict[str, str | bytes] = {}
    env.setdefault("tmp_path", PurePosixPath("."))
    for stmt in walk_statements(func.body):
        if isinstance(stmt, ast.Expr) and record_write(written, stmt.value, env):
            continue
        if is_name_assign(stmt) and written and invokes_graph(stmt.value):
            current = Scenario(
                test=func.name,
                source=source,
                files=dict(written),
                var=stmt.targets[0].id,
            )
            current.aliases[current.var] = set(rule_ids(stmt.value, env))
            scenarios.append(current)
        elif is_name_assign(stmt):
            target = stmt.targets[0].id
            hit = find_analyze_call(stmt.value)
            files = input_files(hit, env) if hit is not None else None
            if files:
                current = Scenario(test=func.name, source=source, files=files, var=target)
                current.aliases[target] = set(rule_ids(stmt.value, env))
                scenarios.append(current)
            elif current is not None and refers_to(stmt.value, current):
                current.aliases[target] = set(rule_ids(stmt.value, env))
            else:
                bind_value(env, target, stmt.value)
        elif (
            isinstance(stmt, ast.Assert)
            and written
            and invokes_graph(stmt.test)
            and (current is None or current.files != written)
        ):
            current = Scenario(test=func.name, source=source, files=dict(written))
            scenarios.append(current)
            visit(current, stmt.test, env, False)
        elif isinstance(stmt, ast.Assert) and current is not None:
            visit(current, stmt.test, env, False)
        elif isinstance(stmt, ast.Expr):
            hit = find_analyze_call(stmt.value)
            files = input_files(hit, env) if hit is not None else None
            if files:
                current = Scenario(test=func.name, source=source, files=files)
                scenarios.append(current)
    return [s for s in scenarios if s.want or s.absent or s.absent_all]


def is_name_assign(stmt: ast.stmt) -> bool:
    return isinstance(stmt, ast.Assign) and len(stmt.targets) == 1 and isinstance(stmt.targets[0], ast.Name)


def record_write(written: dict[str, str | bytes], node: ast.expr, env: dict[str, object]) -> bool:
    if not (isinstance(node, ast.Call) and isinstance(node.func, ast.Attribute) and node.args):
        return False
    if node.func.attr not in {"write_text", "write_bytes"}:
        return False
    try:
        path, content = evaluate(node.func.value, env), evaluate(node.args[0], env)
    except Unresolved:
        return False
    if isinstance(path, PurePosixPath) and isinstance(content, str | bytes):
        written[str(path)] = content
        return True
    return False


def invokes_graph(node: ast.expr) -> bool:
    return any(
        isinstance(sub, ast.Call) and (callee(sub) == "invoke" or any(takes_tmp_path(a) for a in sub.args))
        for sub in ast.walk(node)
    )


def takes_tmp_path(arg: ast.expr) -> bool:
    return (
        isinstance(arg, ast.Name)
        and arg.id == "tmp_path"
        or (isinstance(arg, ast.Call) and any(takes_tmp_path(a) for a in arg.args))
    )


def bind_value(env: dict[str, object], target: str, value: ast.expr) -> None:
    try:
        env[target] = evaluate(value, env)
    except Unresolved:
        env.pop(target, None)


def refers_to(node: ast.AST, sc: Scenario) -> bool:
    names = {n.id for n in ast.walk(node) if isinstance(n, ast.Name)}
    return bool(names & (set(sc.aliases) | {sc.var}) - {""})


def record(sc: Scenario, ids: list[str], neg: bool) -> None:
    for rid in ids:
        (sc.absent if neg else sc.want).add(rid)


def visit(sc: Scenario, test: ast.expr, env: dict[str, object], neg: bool) -> None:
    if isinstance(test, ast.BoolOp):
        for value in test.values:
            visit(sc, value, env, neg)
        return
    if isinstance(test, ast.UnaryOp) and isinstance(test.op, ast.Not):
        visit(sc, test.operand, env, not neg)
        return
    if isinstance(test, ast.Compare) and len(test.comparators) == 1 and isinstance(test.ops[0], ast.In | ast.NotIn):
        ids = strings(test.left, env, RULE_ID)
        if ids and refers_to(test.comparators[0], sc):
            record(sc, ids, neg ^ isinstance(test.ops[0], ast.NotIn))
            return
    if isinstance(test, ast.Compare) and len(test.comparators) == 1 and isinstance(test.ops[0], ast.Eq):
        ids = strings(test.comparators[0], env, RULE_ID)
        if ids and isinstance(test.comparators[0], ast.Set | ast.List) and refers_to(test.left, sc):
            record(sc, ids, neg)
            return
    if not refers_to(test, sc) and not find_analyze_call(test):
        return
    flip = neg ^ polarity_flips(test)
    ids = rule_ids(test, env)
    for name in sorted({n.id for n in ast.walk(test) if isinstance(n, ast.Name)} & set(sc.aliases)):
        ids += sorted(sc.aliases[name])
    record(sc, ids, flip)
    prefixes = startswith_prefixes(test)
    sc.absent.update(f"{p}*" for p in prefixes if flip)
    if not ids and not prefixes and flip:
        sc.absent_all = True


def extract(path: Path) -> list[Scenario]:
    tree = ast.parse(path.read_text(encoding="utf-8"))
    base = module_env(tree)
    found: list[Scenario] = []
    funcs = [n for n in ast.walk(tree) if isinstance(n, ast.FunctionDef) and n.name.startswith("test")]
    for func in funcs:
        for bind in parametrizations(func, base) or [{}]:
            found.extend(collect(func, bind, dict(base), path.name))
    return found


def invisible(ch: str) -> bool:
    if 0xD800 <= ord(ch) <= 0xDFFF:
        return False
    return unicodedata.category(ch) in {
        "Cc",
        "Cf",
        "Zl",
        "Zp",
        "Mn",
        "Me",
        "Co",
        "Cn",
    } or (unicodedata.category(ch) == "Zs" and ch != " ")


def go_string(text: str | bytes) -> str:
    out = ['"']
    if isinstance(text, bytes):
        text = text.decode("latin-1")
        raw = True
    else:
        raw = False
    for ch in text.encode("utf-8", "replace").decode("utf-8", "replace") if not raw else text:
        if raw and (ord(ch) < 0x20 or ord(ch) >= 0x7F) and ch not in "\n\t":
            out.append(f"\\x{ord(ch):02x}")
        elif ch in '"\\':
            out.append("\\" + ch)
        elif ch == "\n":
            out.append("\\n")
        elif ch == "\t":
            out.append("\\t")
        elif invisible(ch):
            out.append(f"\\u{ord(ch):04x}" if ord(ch) <= 0xFFFF else f"\\U{ord(ch):08x}")
        else:
            out.append(ch)
    out.append('"')
    return "".join(out)


def go_name(name: str) -> str:
    return name.removeprefix("test_").replace("_", " ")


def mentions_family(sc: Scenario, wanted: re.Pattern[str]) -> bool:
    words = re.findall(r"[A-Za-z]+\d+", sc.test)
    return any(wanted.fullmatch(w.upper()) for w in words)


def in_family(code: str, wanted: re.Pattern[str]) -> bool:
    if code.endswith("*"):
        return any(wanted.fullmatch(f"{code[:-1]}{i}") for i in range(10))
    return bool(wanted.fullmatch(code))


def skill_path(name: str) -> str:
    rel = PurePosixPath(name)
    return f".ai-rulez/skills/bad/{rel}" if len(rel.parts) > 1 else f".ai-rulez/skills/bad/scripts/{rel.name}"


def placement(files: dict[str, str]) -> str:
    docs = {n: c for n, c in files.items() if n.lower().endswith(".md")}
    others = {n: c for n, c in files.items() if n not in docs}
    skill = f"skill: body({go_string(next(iter(docs.values())))})" if docs else 'skill: body("x\\n")'
    if not others:
        return skill
    entries = ", ".join(f'"{skill_path(n)}": {go_string(c)}' for n, c in others.items())
    return f"files: map[string]string{{{entries}}}, {skill}"


def render(sc: Scenario, wanted: re.Pattern[str], mapping: dict[str, str]) -> str | None:
    want = sorted(c for c in sc.want if in_family(c, wanted))
    absent = sorted(c for c in sc.absent if in_family(c, wanted))
    if not want and not absent and sc.absent_all and mentions_family(sc, wanted):
        absent = sorted(mapping) or [wanted.pattern]
    if not want and not absent:
        return None

    def code(c: str) -> str:
        return mapping.get(c, c.rstrip("*"))

    name = "SKILL.md" if sc.filename.lower().endswith(".md") else Path(sc.filename).name
    fields = [f"name: {go_string(go_name(sc.test))}", placement(sc.files)]
    if want:
        fields.append("want: []string{" + ", ".join(go_string(f"{code(c)}:{name}:0") for c in want) + "}")
    if absent:
        fields.append("absent: []string{" + ", ".join(go_string(code(c)) for c in absent) + "}")
    return "\t\t{" + ", ".join(fields) + "}, // " + f"{sc.source} {sc.filename}"


HEADER = """// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0
//
// Derived from NVIDIA/SkillSpector at commit {upstream} ({sources}).
// Modified by the ai-rulez authors: transcoded from Python tests to Go ruleCase rows.
// See THIRD_PARTY_NOTICES.md.
"""


def transcode(
    tests: Path,
    pattern: str,
    mapping: dict[str, str],
    upstream: str,
    name_filter: str = "",
) -> tuple[str, int]:
    wanted = re.compile(pattern)
    rows: list[str] = []
    sources: set[str] = set()
    seen: set[tuple[str, str]] = set()
    for path in sorted(tests.rglob("test_*.py")):
        for sc in extract(path):
            if name_filter and not re.search(name_filter, f"{sc.test} {sc.source}", re.IGNORECASE):
                continue
            row = render(sc, wanted, mapping)
            key = (row, "")
            if row is None or key in seen:
                continue
            seen.add(key)
            rows.append(row)
            sources.add(f"tests/{path.relative_to(tests)}")
    rows = number_duplicates(rows)
    head = HEADER.format(upstream=upstream, sources=", ".join(sorted(sources)) or "no tests matched")
    body = "\n".join(rows)
    return (
        f"{head}\n// family {pattern}: {len(rows)} rows\nrunRuleCases(t, []ruleCase{{\n{body}\n}})\n",
        len(rows),
    )


def number_duplicates(rows: list[str]) -> list[str]:
    totals = Counter(re.match(r'\t\t\{name: ("[^"]*")', r).group(1) for r in rows)  # type: ignore[union-attr]
    seen: Counter[str] = Counter()
    out = []
    for row in rows:
        name = re.match(r'\t\t\{name: ("[^"]*")', row).group(1)  # type: ignore[union-attr]
        seen[name] += 1
        suffix = f'{name[:-1]} {seen[name]}"' if totals[name] > 1 else name
        out.append(row.replace(name, suffix, 1))
    return out


AR_CODE = re.compile(r"^AR\d{3}$")


def parse_mapping(entries: list[str]) -> dict[str, str]:
    """Parse ``SSID=ARNNN`` entries; raise ValueError naming the first malformed one."""
    mapping = {}
    for entry in entries:
        key, sep, value = (part.strip() for part in entry.partition("="))
        if not sep or not RULE_ID.match(key) or not AR_CODE.match(value):
            raise ValueError(f"--map expects SSID=ARNNN (e.g. DS1=AR044), got {entry!r}")
        mapping[key] = value
    return mapping


SHA_RE = re.compile(r"[0-9a-f]{40}")


class UpstreamError(ValueError):
    """The upstream commit cannot be established reliably."""


def validate_sha(value: str) -> str:
    if not SHA_RE.fullmatch(value):
        raise UpstreamError(f"upstream commit must be a full 40-character lowercase hex sha, got {value!r}")
    return value


def upstream_commit(tests: Path) -> str:
    """Return HEAD of the SkillSpector checkout whose ``tests`` directory is ``tests``.

    The checkout root must be the parent of ``tests``: a copy of the tests that sits inside some other
    repository would otherwise be stamped with that repository's commit.
    """
    parent = tests.resolve().parent
    try:
        top = subprocess.run(
            ["git", "-C", str(parent), "rev-parse", "--show-toplevel"],
            check=True,
            capture_output=True,
            text=True,
        ).stdout.strip()
        head = subprocess.run(
            ["git", "-C", str(parent), "rev-parse", "HEAD"],
            check=True,
            capture_output=True,
            text=True,
        ).stdout.strip()
    except (subprocess.CalledProcessError, FileNotFoundError) as exc:
        raise UpstreamError(f"{parent} is not a git checkout; pass --upstream <40-hex sha>") from exc
    if Path(top).resolve() != parent:
        raise UpstreamError(
            f"{tests} is not at the root of a git checkout (root is {top}); pass --upstream <40-hex sha>"
        )
    return validate_sha(head)


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--tests", type=Path, required=True, help="SkillSpector tests directory")
    ap.add_argument(
        "--rule",
        required=True,
        help="regex fully matching SkillSpector rule ids, e.g. 'DS[1-4]'",
    )
    ap.add_argument(
        "--map",
        action="append",
        default=[],
        metavar="SSID=ARNNN",
        help="rule id mapping (repeatable)",
    )
    ap.add_argument(
        "--only",
        default="",
        help="regex over test and file names to restrict scenarios",
    )
    ap.add_argument(
        "--upstream",
        default="",
        help="full 40-hex upstream commit; defaults to HEAD of the git checkout containing --tests (its parent)",
    )
    ap.add_argument("--out", type=Path, help="write here instead of stdout")
    args = ap.parse_args(argv)
    try:
        mapping = parse_mapping(args.map)
    except ValueError as exc:
        ap.error(str(exc))
    try:
        upstream = validate_sha(args.upstream) if args.upstream else upstream_commit(args.tests)
    except UpstreamError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2
    text, count = transcode(args.tests, args.rule, mapping, upstream, args.only)
    if args.out:
        args.out.write_text(text, encoding="utf-8")
    else:
        sys.stdout.write(text)
    print(f"{args.rule}: {count} rows", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
