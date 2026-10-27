#!/usr/bin/env python3
"""Advance the floating major Action tag (vN) after a successful stable release, without rollback.

Run only in the publish workflow's serialized floating-tag job, from a checkout of the release
commit. Immutable release tags are read, never changed. The major tag is an annotated tag pushed
with --force-with-lease against the value read before the push, so a concurrent move fails instead
of being overwritten. GH_TOKEN needs contents:write on GH_REPO; it reaches git through the
environment, never the command line or .git/config.
"""

import argparse
import base64
import json
import os
import re
import subprocess
import sys

TAGGER = {
    "GIT_COMMITTER_NAME": "github-actions[bot]",
    "GIT_COMMITTER_EMAIL": "41898282+github-actions[bot]@users.noreply.github.com",
}


def stable_version(tag):
    match = re.fullmatch(r"v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)", tag)
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


def git_env(environ=None):
    """The environment for git: the tagger identity and, with GH_TOKEN, an auth header.

    GIT_CONFIG_* first resets http.<server>/.extraheader, so a header a checkout persisted is not
    sent twice, then sets this token's header.
    """
    environ = dict(os.environ if environ is None else environ)
    env = {**environ, **TAGGER}
    token = environ.get("GH_TOKEN", "")
    if token:
        server = environ.get("GITHUB_SERVER_URL", "https://github.com").rstrip("/")
        basic = base64.b64encode(f"x-access-token:{token}".encode()).decode()
        key = f"http.{server}/.extraheader"
        env.update(
            {
                "GIT_CONFIG_COUNT": "2",
                "GIT_CONFIG_KEY_0": key,
                "GIT_CONFIG_VALUE_0": "",
                "GIT_CONFIG_KEY_1": key,
                "GIT_CONFIG_VALUE_1": f"AUTHORIZATION: basic {basic}",
            }
        )
    return env


def run_git(args):
    subprocess.run(["git", *args], env=git_env(), text=True, check=True, capture_output=True)


def advance(repo, tag, expected_sha, api=gh_api, git=run_git, dry_run=False):
    version = stable_version(tag)
    if version is None:
        return "Skip: only stable vX.Y.Z releases advance the major Action tag."
    alias = f"v{version[0]}"
    if not re.fullmatch(r"[0-9a-f]{40}", expected_sha):
        raise ValueError("Expected a full release commit SHA")
    base = f"repos/{repo}"
    release = api(f"{base}/releases/tags/{tag}")
    if release["draft"] or release["prerelease"] or release["tag_name"] != tag:
        raise ValueError(f"{tag} is not a published stable release")
    releases = api(f"{base}/releases?per_page=100", paginate=True)
    for other in releases:
        other_version = stable_version(other["tag_name"])
        if (
            not other["draft"]
            and not other["prerelease"]
            and other_version
            and other_version[0] == version[0]
            and other_version > version
        ):
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
    refs = api(f"{base}/git/matching-refs/tags/{alias}", paginate=True)
    current = next((ref for ref in refs if ref["ref"] == f"refs/tags/{alias}"), None)
    # The lease is the ref's raw value (a tag object for an annotated tag); empty means the push
    # succeeds only while the tag does not exist.
    lease = ""
    if current:
        lease = current["object"]["sha"]
        previous = peel(current["object"])
        if previous == candidate:
            return f"{alias} already points to {tag} ({candidate})."
        comparison = api(f"{base}/compare/{previous}...{candidate}")
        if comparison["status"] == "behind":
            return f"Skip: {alias} already points to a newer commit."
        if comparison["status"] != "ahead":
            raise ValueError(f"Refusing to move {alias} across divergent commit histories")
    verb = "advance" if current else "create"
    if dry_run:
        return f"Would {verb} {alias} at {tag} ({candidate})."
    git(["tag", "--annotate", "--force", "--message", f"{alias}: {tag}", alias, candidate])
    git(
        [
            "push",
            f"--force-with-lease=refs/tags/{alias}:{lease}",
            "origin",
            f"refs/tags/{alias}:refs/tags/{alias}",
        ]
    )
    return f"{'Advanced' if current else 'Created'} {alias} at {tag} ({candidate})."


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--dry-run", action="store_true", help="check live release/ref state without changing the tag")
    args = parser.parse_args()
    try:
        print(advance(os.environ["GH_REPO"], os.environ["RELEASE_TAG"], os.environ["GITHUB_SHA"], dry_run=args.dry_run))
    except (KeyError, ValueError, subprocess.CalledProcessError) as error:
        detail = getattr(error, "stderr", None)
        if detail:
            print(detail, file=sys.stderr)
        print(f"::error::Cannot update the Action tag: {error}", file=sys.stderr)
        sys.exit(1)
