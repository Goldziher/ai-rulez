"""Run paired skill-discovery sessions and emit scored JSONL records."""

import argparse
import hashlib
import json
import os
import platform
import re
import shutil
import subprocess
import sys
import tempfile
import threading
import time
from contextlib import nullcontext
from datetime import UTC, datetime
from pathlib import Path

from generate import LAYOUTS, generate

MODELS = {"claude": "claude-sonnet-4-6", "codex": "gpt-6.1-sol"}
TOOLS = [
    "mcp",
    "--serve-skills",
    "--discover-skills=repo",
    "--read-only",
    "--no-watch",
    "--offline",
    "--tools",
    "find_skill,load_skill",
]
MARKERS = {
    "parcel-dispatch-code": "DISPATCH-CINNABAR-47",
    "parcel-return-code": "RETURN-FELDSPAR-82",
    **{f"station-inspection-{i:03}": f"STATION-OPAL-{i:03}" for i in range(1, 127)},
}
WORKSHEETS = ("services/amber/dispatch.txt", "services/cobalt/dispatch.txt")


def run(argv, cwd, env, prompt, trace):
    """Timestamp the JSONL event arrival, retaining stderr separately."""
    events = []
    start = time.monotonic()
    with (
        trace.open("w") as output,
        trace.with_suffix(".stderr").open("w") as errors,
        subprocess.Popen(
            argv,
            cwd=cwd,
            env=env,
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=errors,
            text=True,
            bufsize=1,
            start_new_session=True,
        ) as process,
    ):
        timed_out = threading.Event()

        def stop():
            timed_out.set()
            try:
                os.killpg(process.pid, 9)
            except ProcessLookupError:
                pass

        timer = threading.Timer(180, stop)
        timer.start()
        try:
            try:
                process.stdin.write(prompt + "\n")
                process.stdin.close()
            except BrokenPipeError:
                pass
            for line in process.stdout:
                elapsed = round(time.monotonic() - start, 3)
                try:
                    event = json.loads(line)
                except json.JSONDecodeError:
                    event = {"type": "non_json", "line": line.rstrip()}
                events.append((elapsed, event))
                print(
                    json.dumps({"elapsed_s": elapsed, "event": event}),
                    file=output,
                    flush=True,
                )
            status = process.wait()
        finally:
            timer.cancel()
        return events, status, timed_out.is_set()


def skill_text(value):
    return json.dumps(value, ensure_ascii=False)


def score(harness, events, task, status, timed_out):
    answer = None
    first_assistant = first_usage = total_usage = first_load = first_selection = None
    seen_skills = stub_visible = None
    session_id = None
    selected = set()
    loaded = set()
    errors = []
    visited = set()
    search_calls = 0
    external_reads = False
    for seconds, event in events:
        kind = event.get("type")
        if kind in {"system", "thread.started"}:
            session_id = event.get("session_id") or event.get("thread_id") or session_id
            if "skills" in event:
                seen_skills = [name for name in event["skills"] if name in MARKERS]
                stub_visible = "dynamic-skills" in event["skills"]
        if (
            kind == "stream_event" and event.get("event", {}).get("type") == "message_start"
        ) and first_assistant is None:
            first_assistant = seconds
            first_usage = event["event"].get("message", {}).get("usage")
        if kind == "assistant" and first_assistant is None:
            first_assistant = seconds
        if kind == "result":
            answer, total_usage = event.get("result"), event.get("usage")
            if event.get("is_error"):
                errors.append(event.get("subtype", "run_error"))
        if kind in {"error", "turn.failed"}:
            errors.append(event.get("error", {}).get("message") or event.get("message", kind))
        item = event.get("item", {})
        if kind == "item.completed" and item.get("type") == "agent_message":
            answer = item.get("text")
            if first_assistant is None:
                first_assistant = seconds
        if kind == "turn.completed":
            total_usage = event.get("usage")
        action_text = ""
        result_text = ""
        if harness == "claude":
            content = event.get("message", {}).get("content", [])
            if isinstance(content, list):
                action_text = skill_text([block for block in content if block.get("type") == "tool_use"])
                if kind == "user":
                    result_text = skill_text(content)
        else:
            if kind == "item.started":
                action_text = skill_text({key: item.get(key) for key in ("command", "arguments")})
            if kind == "item.completed" and item.get("type") in {
                "command_execution",
                "mcp_tool_call",
            }:
                result_text = skill_text({key: item.get(key) for key in ("aggregated_output", "result")})
        if '"name": "mcp__skills__find_skill"' in action_text or (
            kind == "item.started" and item.get("tool") == "find_skill"
        ):
            search_calls += 1
        if "/catalog/" in action_text or ("../catalog" in action_text):
            external_reads = True
        for path in WORKSHEETS:
            if path in action_text:
                visited.add(path)
        for name, marker in MARKERS.items():
            if name in action_text:
                selected.add(name)
                if name == task.get("expected_skill") and first_selection is None:
                    first_selection = seconds
            if marker in result_text:
                loaded.add(name)
                if name == task.get("expected_skill") and first_load is None:
                    first_load = seconds
    expected = task.get("expected_answer")
    exact = isinstance(answer, str) and answer.strip() == expected
    marker_found = bool(answer and expected and re.search(rf"(?<![\w-]){re.escape(expected)}(?![\w-])", answer))
    return {
        "session_id": session_id,
        "answer": answer,
        "exact_match": exact,
        "answer_contains_code": marker_found,
        "selected": sorted(selected),
        "loaded": sorted(loaded),
        "startup_usage": first_usage,
        "turn_usage": total_usage,
        "first_assistant_s": first_assistant,
        "first_skill_selection_s": first_selection,
        "first_skill_use_s": first_load,
        "visible_fixture_skills": seen_skills,
        "stub_visible": stub_visible,
        "exit_code": status,
        "timed_out": timed_out,
        "errors": errors,
        "duration_s": events[-1][0] if events else None,
        "scope_paths_in_tool_calls": sorted(visited),
        "mcp_search_calls": search_calls,
        "external_catalog_read_attempt": external_reads,
    }


def command(harness, config, model, persistent, session_id):
    if harness == "claude":
        args = [
            "claude",
            "-p",
            "--verbose",
            "--output-format",
            "stream-json",
            "--include-partial-messages",
            "--strict-mcp-config",
            "--setting-sources",
            "project",
            "--model",
            model,
            "--effort",
            "low",
            "--tools",
            "Read,Skill,ToolSearch",
            "--allowedTools",
            "mcp__skills__*",
            "--settings",
            '{"disableAllHooks":true,"autoMemoryEnabled":false}',
        ]
        if config:
            args += ["--mcp-config", str(config)]
        if not persistent:
            args.append("--no-session-persistence")
        if session_id:
            args += ["--resume", session_id]
        return args
    args = ["codex", "exec", "--sandbox", "read-only", "--ignore-rules"]
    if session_id:
        args.extend(["resume", session_id])
    elif not persistent:
        args.append("--ephemeral")
    return args + ["--json", "-m", model, "-c", 'model_reasoning_effort="low"', "-"]


def setup_mcp(root, catalog, binary, codex_home, workspace):
    launcher = root / "serve.sh"
    launcher.write_text('#!/bin/sh\ncd "$1" || exit\nshift\nexec "$@"\n')
    values = [str(launcher), str(catalog), str(binary), *TOOLS]
    if codex_home is None:
        cfg = root / "mcp.json"
        cfg.write_text(json.dumps({"mcpServers": {"skills": {"command": "/bin/sh", "args": values}}}))
        return cfg
    cfg = Path(codex_home) / "config.toml"
    cfg.write_text(
        '[mcp_servers.skills]\ncommand = "/bin/sh"\nargs = '
        + json.dumps(values)
        + "\n"
        + f'[projects.{json.dumps(str(workspace.resolve()))}]\ntrust_level = "trusted"\n'
    )
    return cfg


def build_stub(binary, root):
    authored = root / ".ai-rulez"
    placeholder = authored / "skills/example/SKILL.md"
    placeholder.parent.mkdir(parents=True)
    placeholder.write_text("---\ndescription: Placeholder to generate the discovery stub.\n---\n\n# Example\n")
    (authored / "config.toml").write_text(
        'version = "5.0"\nname = "skill-discovery-benchmark"\npresets = ["claude", "codex"]\n'
        '[skills]\ndelivery = "served"\n'
        '[[mcp_servers]]\nname = "skills"\ncommand = "ai-rulez"\nargs = ["mcp", "--serve-skills"]\n'
    )
    subprocess.run(["git", "init", "-q"], cwd=root, check=True)
    subprocess.run([binary, "generate", "--yes"], cwd=root, check=True, stdout=sys.stderr)


def record_attempt(args, base, case, task, arm, directory):
    directory.mkdir(parents=True)
    catalog = directory / "catalog"
    workspace = directory / "workspace"
    native = f"{args.harness}-native"
    control = task["id"] in {"startup-native-empty", "startup-mcp-empty"}
    fixture = "mcp-workspace" if arm == "mcp" or control else native
    shutil.copytree(case / fixture, workspace, ignore=shutil.ignore_patterns(".git"))
    if args.mcp_stub and arm == "mcp":
        stub = LAYOUTS[native] / "dynamic-skills/SKILL.md"
        (workspace / stub).parent.mkdir(parents=True)
        shutil.copy2(args.output / "stub-source" / stub, workspace / stub)
    subprocess.run(["git", "init", "-q"], cwd=workspace, check=True)
    if arm == "mcp":
        if control:
            catalog.mkdir()
        else:
            shutil.copytree(case / "mcp-catalog", catalog)
        subprocess.run(["git", "init", "-q"], cwd=catalog, check=True)
    auth_home = tempfile.TemporaryDirectory(prefix="skillbench-codex-") if args.harness == "codex" else nullcontext()
    with auth_home as codex_home:
        env = {**os.environ, "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1"}
        if codex_home:
            shutil.copy2(args.auth_file, Path(codex_home) / "auth.json")
            env["CODEX_HOME"] = codex_home
        config = setup_mcp(directory, catalog, args.binary, codex_home, workspace) if arm == "mcp" else None
        run_turns(args, base, task, arm, directory, workspace, env, config)


def run_turns(args, base, task, arm, directory, workspace, env, config):
    turns = task.get("turns", [task])
    session_id = None
    for turn_num, turn in enumerate(turns, 1):
        trace = directory / f"turn-{turn_num}.jsonl"
        argv = command(args.harness, config, args.model, len(turns) > 1, session_id)
        events, status, timed_out = run(argv, workspace, env, turn["prompt"], trace)
        result = score(args.harness, events, turn, status, timed_out)
        print(
            json.dumps(
                {
                    **base,
                    "task": task["id"],
                    "arm": arm,
                    "turn": turn_num,
                    "expected_answer": turn["expected_answer"],
                    "expected_skill": turn.get("expected_skill"),
                    "argv": argv,
                    "trace": str(trace.relative_to(args.output)),
                    "session_resumed": session_id == result["session_id"] if session_id is not None else None,
                    **result,
                }
            ),
            flush=True,
        )
        session_id = result["session_id"]
        if not session_id or status or timed_out:
            break


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("harness", choices=MODELS)
    parser.add_argument("binary", type=Path)
    parser.add_argument(
        "output",
        type=Path,
        help="new directory for fixtures and private raw transcripts",
    )
    parser.add_argument("--pairs", type=int, default=1)
    parser.add_argument("--size", choices=["small", "large"], action="append")
    parser.add_argument("--layout", choices=["flat", "nested"], action="append")
    parser.add_argument("--task", action="append")
    parser.add_argument("--model")
    parser.add_argument(
        "--mcp-stub",
        action="store_true",
        help="use the generated dynamic-skills stub in MCP arms",
    )
    parser.add_argument("--auth-file", type=Path, default=Path.home() / ".codex/auth.json")
    args = parser.parse_args()
    args.binary, args.output = args.binary.resolve(), args.output.resolve()
    args.model = args.model or MODELS[args.harness]
    args.output.mkdir(mode=0o700)
    if args.mcp_stub:
        build_stub(args.binary, args.output / "stub-source")
    fixture = args.output / "fixture"
    generate(fixture)
    manifest = json.loads((fixture / "tasks.json").read_text())
    metadata = {
        "harness": args.harness,
        "model": args.model,
        "effort": "low",
        "fixture_version": manifest["fixture_version"],
        "version": subprocess.check_output([args.harness, "--version"], text=True).strip(),
        "ai_rulez_version": subprocess.check_output([args.binary, "version"], text=True).strip(),
        "ai_rulez_sha256": hashlib.file_digest(args.binary.open("rb"), "sha256").hexdigest(),
        "platform": platform.platform(),
        "started_utc": datetime.now(UTC).isoformat(),
        "fixture_revision": subprocess.check_output(["git", "rev-parse", "HEAD"], text=True).strip(),
        "invocation": sys.argv,
        "cache_policy": "service defaults, fresh local session, no cache reset",
        "mcp_stub": args.mcp_stub,
    }
    (args.output / "metadata.json").write_text(json.dumps(metadata, indent=2) + "\n")
    attribution = [
        {
            "id": name,
            "layouts": ["flat", "nested"],
            "prompt": "Reply exactly READY.",
            "expected_answer": "READY",
        }
        for name in ("startup", "startup-native-empty", "startup-mcp-empty")
    ]
    for size in args.size or manifest["sizes"]:
        for layout in args.layout or ("flat", "nested"):
            for task in [*manifest["tasks"], *attribution]:
                if layout not in task["layouts"] or (args.task and task["id"] not in args.task):
                    continue
                for pair in range(1, args.pairs + 1):
                    arms = ("native", "mcp") if pair % 2 else ("mcp", "native")
                    for order, arm in enumerate(arms, 1):
                        if task["id"].endswith("-empty") and task["id"] != f"startup-{arm}-empty":
                            continue
                        base = {
                            "harness": args.harness,
                            "size": size,
                            "layout": layout,
                            "pair": pair,
                            "order": order,
                        }
                        directory = args.output / f"{size}-{layout}-{task['id']}-{pair}-{arm}"
                        record_attempt(args, base, fixture / size / layout, task, arm, directory)


if __name__ == "__main__":
    main()
