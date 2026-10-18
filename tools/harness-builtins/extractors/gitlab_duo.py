"""GitLab Duo CLI: slash commands declared by the handlers of packages/cli/app/src/slash_commands.

Each handler class has `command = { name: '/exit', aliases: ['/quit'], ... }`. The repository is on
gitlab.com, so the files are read through the GitLab API (ctx.get) at the commit the pin names (ref).
"""

import json
import re
import urllib.parse

PROJECT = "gitlab-org/editor-extensions/gitlab-lsp"
DIR = "packages/cli/app/src/slash_commands/handlers"
API = "https://gitlab.com/api/v4/projects/" + urllib.parse.quote(PROJECT, safe="")
RAW = f"https://gitlab.com/{PROJECT}/-/raw"
NAME = re.compile(r"""\bname:\s*'/([\w-]+)'""")
ALIASES = re.compile(r"""\baliases:\s*\[([^\]]*)\]""")


def handler_files(listing: str) -> list[str]:
    """The non-test handler sources of a repository tree listing."""
    paths = [e["path"] for e in json.loads(listing) if e.get("type") == "blob"]
    return sorted(p for p in paths if p.endswith(".ts") and not p.endswith(".test.ts"))


def strip_comments(text: str) -> str:
    text = re.sub(r"/\*.*?\*/", "", text, flags=re.DOTALL)
    return re.sub(r"(?m)^\s*//.*$", "", text)


def parse_handler(text: str) -> tuple[str, list[str]] | None:
    """(name, aliases) of the command the handler declares, None when it declares no command."""
    text = strip_comments(text)
    m = NAME.search(text)
    if not m:
        return None
    a = ALIASES.search(text)
    aliases = re.findall(r"""'/([\w-]+)'""", a.group(1)) if a else []
    return m.group(1), aliases


def extract(ctx) -> dict:
    ref = ctx.ref
    if not ref:
        raise ValueError("the pin needs ref = the commit of gitlab-lsp")
    listing = ctx.get(f"{API}/repository/tree?path={DIR}&ref={ref}&per_page=100")
    names, aliases = [], {}
    for path in handler_files(listing):
        got = parse_handler(ctx.get(f"{RAW}/{ref}/{path}"))
        if got is None:
            continue
        names.append(got[0])
        if got[1]:
            aliases[got[0]] = got[1]
    if not names:
        raise ValueError("no command handlers found")
    return {"command": {"names": names, "aliases": aliases}}
