"""Fetching sources and the context an extractor reads them through.

Only https URLs are fetched, and only to public hosts: raw.githubusercontent.com and
api.github.com for repositories, and the documentation pages a pin names. A GITHUB_TOKEN
in the environment is sent to the two GitHub hosts and nowhere else.
"""

import json
import os
import re
import ssl
import subprocess
import urllib.error
import urllib.parse
import urllib.request

USER_AGENT = "ai-rulez-harness-builtins"
GITHUB_HOSTS = {"raw.githubusercontent.com", "api.github.com"}
TIMEOUT = 60


class FetchError(Exception):
    pass


def _request(url: str) -> urllib.request.Request:
    p = urllib.parse.urlsplit(url)
    if p.scheme != "https" or not p.hostname or p.username or p.password:
        raise FetchError(f"refusing to fetch {url}: only plain https URLs")
    headers = {"User-Agent": USER_AGENT}
    token = os.environ.get("GITHUB_TOKEN")
    if token and p.hostname in GITHUB_HOSTS:
        headers["Authorization"] = f"Bearer {token}"
    return urllib.request.Request(url, headers=headers)


def http_get(url: str) -> str:
    req = _request(url)
    try:
        with urllib.request.urlopen(req, timeout=TIMEOUT) as resp:
            return resp.read().decode("utf-8", "replace")
    except urllib.error.HTTPError as e:
        raise FetchError(f"{url}: HTTP {e.code}") from e
    except (urllib.error.URLError, ssl.SSLError, TimeoutError) as e:
        return _curl(url, e)


def _curl(url: str, cause: Exception) -> str:
    # Some Python builds ship without a usable trust store; curl uses the system one. The
    # request goes through a curl config on stdin so a token never appears in the argument list.
    req = _request(url)
    config = [f'url = "{url}"', f'user-agent = "{USER_AGENT}"']
    if "Authorization" in req.headers:
        config.append(f'header = "Authorization: {req.headers["Authorization"]}"')
    try:
        out = subprocess.run(
            ["curl", "-sSLf", "-m", str(TIMEOUT), "-K", "-"],
            input="\n".join(config).encode(),
            capture_output=True,
            check=True,
        )
    except (OSError, subprocess.CalledProcessError) as e:
        detail = e.stderr.decode("utf-8", "replace").strip() if isinstance(e, subprocess.CalledProcessError) else str(e)
        raise FetchError(f"{url}: {detail or cause} (rate limited? set GITHUB_TOKEN)") from e
    return out.stdout.decode("utf-8", "replace")


class Context:
    """What an extractor reads: files of the pinned repository and public pages.

    get is replaceable, so tests run an extractor against saved files.
    """

    def __init__(self, repo: str = "", ref: str = "", get=http_get):
        self.repo, self.ref, self._get, self._cache = repo, ref, get, {}

    def get(self, url: str) -> str:
        if url not in self._cache:
            self._cache[url] = self._get(url)
        return self._cache[url]

    def raw(self, path: str, repo: str = "", ref: str = "") -> str:
        """The text of a file of the pinned repository (or another repo/ref)."""
        repo, ref = repo or self.repo, ref or self.ref
        if not repo or not ref:
            raise FetchError("raw() needs a repo and ref in the pin")
        return self.get(f"https://raw.githubusercontent.com/{repo}/{ref}/{path.lstrip('/')}")

    def tree(self, prefix: str = "", repo: str = "", ref: str = "") -> list[str]:
        """Sorted file paths of the repository at the pin that start with prefix."""
        repo, ref = repo or self.repo, ref or self.ref
        if not repo or not ref:
            raise FetchError("tree() needs a repo and ref in the pin")
        body = self.get(f"https://api.github.com/repos/{repo}/git/trees/{ref}?recursive=1")
        data = json.loads(body)
        if data.get("truncated"):
            raise FetchError(f"{repo}@{ref}: tree listing is truncated")
        return sorted(e["path"] for e in data["tree"] if e["type"] == "blob" and e["path"].startswith(prefix))


FRONTMATTER = re.compile(r"\A---\s*\n(.*?)\n---\s*(?:\n|\Z)", re.DOTALL)


def frontmatter_field(text: str, key: str) -> str | None:
    """A scalar key of a markdown file's YAML front matter (no YAML parser: one-line values)."""
    m = FRONTMATTER.match(text)
    if not m:
        return None
    for line in m.group(1).splitlines():
        k, sep, v = line.partition(":")
        if sep and k.strip() == key:
            v = v.strip()
            if len(v) >= 2 and v[0] == v[-1] and v[0] in "\"'":
                v = v[1:-1]
            return v
    return None
