"""Check that a quote occurs verbatim on the page its source names."""

import re

from hb import fetch


def raw_url(url: str) -> str:
    """The URL to fetch for a source URL: GitHub blob URLs are read from raw.githubusercontent.com."""
    m = re.match(r"https://github\.com/([^/]+/[^/]+)/blob/([^/]+)/(.*?)(#.*)?$", url)
    if m:
        return f"https://raw.githubusercontent.com/{m.group(1)}/{m.group(2)}/{m.group(3)}"
    return url.split("#")[0]


def squash(s: str) -> str:
    return re.sub(r"\s+", " ", s)


def has_quote(page: str, quote: str) -> bool:
    return squash(quote) in squash(page)


def verify(url: str, quote: str, get=fetch.http_get) -> bool:
    return has_quote(get(raw_url(url)), quote)
