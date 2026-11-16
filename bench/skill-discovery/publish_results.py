"""Extract publishable measurements from private runs without copying transcripts."""

import argparse
import json
from pathlib import Path

from run import score


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


def public_record(run, log, record):
    result = score(
        record["harness"],
        list(read_events(log.with_suffix("") / record["trace"])),
        record,
        record["exit_code"],
        record["timed_out"],
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
        series = "generated-stub" if metadata.get("mcp_stub") else "bare-mcp"
        for line in log.read_text().splitlines():
            print(json.dumps(public_record(series, log, json.loads(line)), separators=(",", ":")))


if __name__ == "__main__":
    main()
