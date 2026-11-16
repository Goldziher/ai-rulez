"""Generate versioned, offline skill-discovery benchmark fixtures."""

import argparse
import json
from pathlib import Path

VERSION = 1
CATALOG_SIZES = {"small": 8, "large": 128}
TASK_ROOT = Path("services/amber")
SIBLING_ROOT = Path("services/cobalt")
TASK_FILE = TASK_ROOT / "dispatch.txt"
SIBLING_FILE = SIBLING_ROOT / "dispatch.txt"
TARGET_NAME = "parcel-dispatch-code"
TARGET_MARKER = "DISPATCH-CINNABAR-47"
SIBLING_MARKER = "RETURN-FELDSPAR-82"
DESCRIPTION = "Look up the parcel dispatch code for an outbound dispatch worksheet."
SIBLING_DESCRIPTION = "Look up the parcel return code for an inbound returns worksheet."
LAYOUTS = {
    "claude-native": Path(".claude/skills"),
    "codex-native": Path(".agents/skills"),
    "mcp-catalog": Path("skills"),
}


def write(path: Path, content: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_bytes(content.encode("utf-8"))


def skill(path: Path, name: str, description: str, body: str) -> None:
    write(
        path / name / "SKILL.md",
        f"---\nname: {name}\ndescription: {description}\n---\n\n# {name}\n\n{body}\n",
    )


def workspace(directory: Path) -> None:
    write(directory / TASK_FILE, "Outbound parcel dispatch worksheet\n")
    write(directory / SIBLING_FILE, "Inbound parcel returns worksheet\n")


def catalog(directory: Path, skills: Path, count: int, nested: bool) -> None:
    target = TASK_ROOT if nested else Path()
    sibling = SIBLING_ROOT if nested else Path()
    skill(
        directory / target / skills,
        TARGET_NAME,
        DESCRIPTION,
        f"For an outbound dispatch worksheet, report the exact dispatch code `{TARGET_MARKER}`.",
    )
    skill(
        directory / sibling / skills,
        "parcel-return-code",
        SIBLING_DESCRIPTION,
        f"For an inbound returns worksheet, report the exact return code `{SIBLING_MARKER}`.",
    )
    for number in range(1, count - 1):
        scope = sibling if number % 2 else Path()
        skill(
            directory / scope / skills,
            f"station-inspection-{number:03}",
            f"Look up the inspection record code for instrument station {number:03}.",
            f"For instrument station {number:03}, report the inspection code `STATION-OPAL-{number:03}`.",
        )


def task_manifest() -> dict:
    request_code = "What is the parcel dispatch code for the worksheet at {}?"
    missing = "If no applicable repository guidance supplies a code, answer exactly NO-CODE."
    return {
        "fixture_version": VERSION,
        "sizes": CATALOG_SIZES,
        "tasks": [
            {
                "id": "dispatch",
                "layouts": ["flat", "nested"],
                "prompt": f"{request_code.format(TASK_FILE.as_posix())} {missing}",
                "expected_answer": TARGET_MARKER,
                "expected_skill": TARGET_NAME,
            },
            {
                "id": "sibling-walk",
                "layouts": ["flat", "nested"],
                "turns": [
                    {
                        "prompt": f"{request_code.format(TASK_FILE.as_posix())} {missing}",
                        "expected_answer": TARGET_MARKER,
                        "expected_skill": TARGET_NAME,
                    },
                    {
                        "prompt": f"What is the parcel return code for {SIBLING_FILE.as_posix()}? {missing}",
                        "expected_answer": SIBLING_MARKER,
                        "expected_skill": "parcel-return-code",
                    },
                ],
            },
            {
                "id": "wrong-scope",
                "layouts": ["nested"],
                "prompt": f"{request_code.format(SIBLING_FILE.as_posix())} {missing}",
                "expected_answer": "NO-CODE",
                "expected_skill": None,
            },
            {
                "id": "unrelated",
                "layouts": ["flat", "nested"],
                "prompt": "What is 17 plus 25? Reply with the number only.",
                "expected_answer": "42",
                "expected_skill": None,
            },
        ],
    }


def generate(output: Path) -> None:
    output.mkdir(parents=True)
    write(output / "tasks.json", json.dumps(task_manifest(), indent=2) + "\n")
    for size, count in CATALOG_SIZES.items():
        for layout in ("flat", "nested"):
            case = output / size / layout
            for arm, skills in LAYOUTS.items():
                workspace(case / arm)
                catalog(case / arm, skills, count, nested=layout == "nested")
            workspace(case / "mcp-workspace")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("output", type=Path, help="new directory outside the harness workspaces")
    args = parser.parse_args()
    try:
        generate(args.output)
    except FileExistsError:
        parser.error(f"output already exists: {args.output}")


if __name__ == "__main__":
    main()
