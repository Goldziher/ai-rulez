"""Extract publishable measurements from private runs without copying transcripts."""

import argparse
import json
from pathlib import Path

from run import WORKSHEETS, score


def usage(source, harness):
    if source is None:
        return None
    cache_read = source.get("cache_read_input_tokens", source.get("cached_input_tokens", 0))
    cache_write = source.get("cache_creation_input_tokens", source.get("cache_write_input_tokens", 0))
    input_tokens = source.get("input_tokens")
    if input_tokens is not None and harness == "claude":
        input_tokens += cache_read + cache_write
    return {
        "input": input_tokens,
        "output": source.get("output_tokens"),
        "cache_read": cache_read,
        "cache_write": cache_write,
    }


def outcome(result):
    if result["timed_out"]:
        return "timeout"
    if result["errors"] or result["exit_code"]:
        return "harness-error"
    if result["external_catalog_read_attempt"]:
        return "external-catalog-read-attempt"
    if result["exact_match"]:
        return "exact"
    if result["answer_contains_code"]:
        return "contains-expected"
    if result["answer"] is None:
        return "no-answer"
    if result["answer"].strip() == "NO-CODE":
        return "no-code"
    return "unexpected-answer"


def read_events(path):
    for line in path.read_text().splitlines():
        event = json.loads(line)
        yield event["elapsed_s"], event["event"]


def claude_discovery_metrics(events):
    actions, results = [], []
    for _, event in events:
        content = event.get("message", {}).get("content", [])
        if not isinstance(content, list):
            continue
        if event.get("type") == "assistant":
            actions.extend(block for block in content if block.get("type") == "tool_use")
        if event.get("type") == "user":
            results.extend(block for block in content if block.get("type") == "tool_result")

    lookup = "mcp__skills__find_skill"
    load = "mcp__skills__load_skill"
    mcp_ids = {action["id"] for action in actions if action.get("name") in {lookup, load}}
    lookup_paths = [action.get("input", {}).get("path") for action in actions if action.get("name") == lookup]
    return {
        "mcp_search_calls": len(lookup_paths),
        "mcp_load_calls": sum(action.get("name") == load for action in actions),
        "mcp_tool_errors": sum(
            bool(result.get("is_error")) and result.get("tool_use_id") in mcp_ids for result in results
        ),
        "mcp_lookup_worksheet_paths": sorted(path for path in WORKSHEETS if path in lookup_paths),
        "tool_search_calls": sum(action.get("name") == "ToolSearch" for action in actions),
        "stub_invocations": sum(
            action.get("name") == "Skill" and action.get("input", {}).get("skill") == "dynamic-skills"
            for action in actions
        ),
    }


def public_record(run, log, record, binary_sha):
    events = list(read_events(log.with_suffix("") / record["trace"]))
    result = score(record["harness"], events, record, record["exit_code"], record["timed_out"])
    instrumentation = (
        {"ai_rulez_sha256": binary_sha, **claude_discovery_metrics(events)}
        if run.startswith("claude-discovery-")
        else {}
    )
    return {
        "series": run,
        **{
            key: record[key]
            for key in (
                "harness",
                "size",
                "layout",
                "task",
                "pair",
                "order",
                "arm",
                "turn",
                "expected_skill",
                "session_resumed",
            )
        },
        **{
            key: result[key]
            for key in (
                "stub_visible",
                "selected",
                "loaded",
                "first_assistant_s",
                "first_skill_selection_s",
                "first_skill_use_s",
                "duration_s",
                "scope_paths_in_tool_calls",
                "mcp_search_calls",
                "external_catalog_read_attempt",
            )
        },
        **instrumentation,
        "visible_fixture_skill_count": None
        if result["visible_fixture_skills"] is None
        else len(result["visible_fixture_skills"]),
        "startup": usage(result["startup_usage"], record["harness"]),
        "turn_usage": usage(result["turn_usage"], record["harness"]),
        "outcome": outcome(result),
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "logs",
        nargs="+",
        type=Path,
        help="JSONL runner outputs beside their private run directories",
    )
    args = parser.parse_args()
    for log in args.logs:
        metadata = json.loads((log.with_suffix("") / "metadata.json").read_text())
        variant = metadata.get("claude_discovery")
        series = (
            f"claude-discovery-{variant}" if variant else "generated-stub" if metadata.get("mcp_stub") else "bare-mcp"
        )
        for line in log.read_text().splitlines():
            print(
                json.dumps(
                    public_record(series, log, json.loads(line), metadata.get("ai_rulez_sha256")), separators=(",", ":")
                )
            )


if __name__ == "__main__":
    main()
