#!/usr/bin/env python3
"""Advance the v5 Action ref after a successful stable release, without rollback.

Run only in the publish workflow's serialized floating-tag job. Immutable release
tags are read, never changed. GH_TOKEN needs contents:write on GH_REPO.
"""

import argparse
import json
import os
import re
import subprocess
import sys


def stable_version(tag):
    match = re.fullmatch(r"v5\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)", tag)
    return tuple(map(int, match.groups())) if match else None


def gh_api(endpoint, method="GET", payload=None, paginate=False):
    command = ["gh", "api", "--method", method, endpoint]
    if paginate:
        command.extend(["--paginate", "--slurp"])
    if payload is not None:
        command.extend(["--input", "-"])
    result = subprocess.run(
        command,
        input=json.dumps(payload) if payload is not None else None,
        text=True,
        check=True,
        capture_output=True,
    )
    data = json.loads(result.stdout)
    return [item for page in data for item in page] if paginate else data


def advance(repo, tag, expected_sha, api=gh_api, dry_run=False):
    version = stable_version(tag)
    if version is None:
        return "Skip: only stable v5.x.y releases advance v5."
    if not re.fullmatch(r"[0-9a-f]{40}", expected_sha):
        raise ValueError("Expected a full release commit SHA")
    base = f"repos/{repo}"
    release = api(f"{base}/releases/tags/{tag}")
    if release["draft"] or release["prerelease"] or release["tag_name"] != tag:
        raise ValueError(f"{tag} is not a published stable release")
    releases = api(f"{base}/releases?per_page=100", paginate=True)
    for other in releases:
        other_version = stable_version(other["tag_name"])
        if not other["draft"] and not other["prerelease"] and other_version and other_version > version:
            return f"Skip: newer stable release {other['tag_name']} already exists."

    def peel(obj):
        # Annotated tags can refer to other annotated tags; bound traversal.
        for _ in range(16):
            if obj["type"] == "commit":
                return obj["sha"]
            if obj["type"] != "tag":
                raise ValueError("Action tags must resolve to commits")
            obj = api(f"{base}/git/tags/{obj['sha']}")["object"]
        raise ValueError("Too many nested annotated tags")

    candidate = peel(api(f"{base}/git/ref/tags/{tag}")["object"])
    if candidate != expected_sha:
        raise ValueError(f"{tag} no longer matches the publishing run's commit")
    refs = api(f"{base}/git/matching-refs/tags/v5", paginate=True)
    current = next((ref for ref in refs if ref["ref"] == "refs/tags/v5"), None)
    if current:
        previous = peel(current["object"])
        if previous == candidate:
            return f"v5 already points to {tag} ({candidate})."
        comparison = api(f"{base}/compare/{previous}...{candidate}")
        if comparison["status"] == "behind":
            return "Skip: v5 already points to a newer commit."
        if comparison["status"] != "ahead":
            raise ValueError("Refusing to move v5 across divergent commit histories")
        if dry_run:
            return f"Would advance v5 to {tag} ({candidate})."
        api(f"{base}/git/refs/tags/v5", method="PATCH", payload={"sha": candidate, "force": True})
    else:
        if dry_run:
            return f"Would create v5 at {tag} ({candidate})."
        api(f"{base}/git/refs", method="POST", payload={"ref": "refs/tags/v5", "sha": candidate})
    return f"Advanced v5 to {tag} ({candidate})."


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--dry-run", action="store_true", help="check live release/ref state without changing v5")
    args = parser.parse_args()
    try:
        print(advance(os.environ["GH_REPO"], os.environ["RELEASE_TAG"], os.environ["GITHUB_SHA"], dry_run=args.dry_run))
    except (KeyError, ValueError, subprocess.CalledProcessError) as error:
        print(f"::error::Cannot update the Action tag: {error}", file=sys.stderr)
        sys.exit(1)
