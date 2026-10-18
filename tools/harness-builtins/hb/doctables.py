"""Read the tables of a documentation page, markdown or rendered HTML.

Both readers return the same shape: a list of {"heading", "headers", "rows"} where heading is the
text of the nearest preceding heading and every cell is a string. Inline code is kept in backticks
(`<code>x</code>` becomes `x`), so slash_names() reads a cell of either source the same way.
"""

import re
from html.parser import HTMLParser

SEPARATOR = re.compile(r"^\s*\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)*\|?\s*$")
SLASH = re.compile(r"`/([A-Za-z0-9][\w:.-]*)")


def split_row(line: str) -> list[str]:
    """Cells of a markdown table row. A pipe inside backticks or escaped as \\| is not a separator."""
    line = line.strip()
    line = line.removeprefix("|")
    if line.endswith("|") and not line.endswith("\\|"):
        line = line[:-1]
    cells, cur, in_code, i = [], [], False, 0
    while i < len(line):
        ch = line[i]
        if ch == "\\" and line[i + 1 : i + 2] == "|":
            cur.append("|")
            i += 2
            continue
        if ch == "`":
            in_code = not in_code
        if ch == "|" and not in_code:
            cells.append("".join(cur).strip())
            cur = []
        else:
            cur.append(ch)
        i += 1
    cells.append("".join(cur).strip())
    return cells


def md_tables(text: str) -> list[dict]:
    lines, tables, heading, i = text.splitlines(), [], "", 0
    while i < len(lines):
        line = lines[i]
        m = re.match(r"^#{1,6}\s+(.*?)\s*#*\s*$", line)
        if m:
            heading = m.group(1)
        elif line.lstrip().startswith("|") and i + 1 < len(lines) and SEPARATOR.match(lines[i + 1]):
            headers, rows, i = split_row(line), [], i + 2
            while i < len(lines) and lines[i].lstrip().startswith("|"):
                rows.append(split_row(lines[i]))
                i += 1
            tables.append({"heading": heading, "headers": headers, "rows": rows})
            continue
        i += 1
    return tables


class _Tables(HTMLParser):
    def __init__(self):
        super().__init__(convert_charrefs=True)
        self.tables, self.heading = [], ""
        self._h = None
        self._table = None
        self._row = None
        self._cell = None
        self._skip = 0

    def handle_starttag(self, tag, attrs):
        if tag in ("script", "style", "svg"):
            self._skip += 1
        elif re.fullmatch(r"h[1-6]", tag):
            self._h = []
        elif tag == "table":
            self._table = {"heading": self.heading, "headers": [], "rows": []}
        elif self._table is not None and tag == "tr":
            self._row = []
        elif self._row is not None and tag in ("td", "th"):
            self._cell = []
        elif self._cell is not None and tag == "code":
            self._cell.append("`")
        elif self._cell is not None and tag == "br":
            self._cell.append(" ")

    def handle_endtag(self, tag):
        if tag in ("script", "style", "svg"):
            self._skip = max(0, self._skip - 1)
        elif re.fullmatch(r"h[1-6]", tag) and self._h is not None:
            self.heading = re.sub(r"\s+", " ", "".join(self._h)).strip()
            self._h = None
        elif self._cell is not None and tag == "code":
            self._cell.append("`")
        elif self._row is not None and tag in ("td", "th"):
            self._row.append(re.sub(r"\s+", " ", "".join(self._cell)).strip())
            self._cell = None
        elif self._table is not None and tag == "tr" and self._row is not None:
            if not self._table["headers"]:
                self._table["headers"] = self._row
            else:
                self._table["rows"].append(self._row)
            self._row = None
        elif tag == "table" and self._table is not None:
            self.tables.append(self._table)
            self._table = None

    def handle_data(self, data):
        if self._skip:
            return
        if self._cell is not None:
            self._cell.append(data)
        if self._h is not None:
            self._h.append(data)


def html_tables(text: str) -> list[dict]:
    parser = _Tables()
    parser.feed(text)
    parser.close()
    return parser.tables


def find_tables(tables: list[dict], heading: str = "", headers: list[str] | None = None) -> list[dict]:
    """The tables whose heading contains `heading` and whose first header cells equal `headers`."""
    out = []
    for t in tables:
        if heading.casefold() not in t["heading"].casefold():
            continue
        if headers and [h.casefold() for h in t["headers"][: len(headers)]] != [h.casefold() for h in headers]:
            continue
        out.append(t)
    return out


def slash_names(cell: str) -> list[str]:
    """Names of the backticked /commands in a cell, in order, without arguments."""
    return SLASH.findall(cell)


REVERSE_ALIAS = re.compile(
    r"(?:\balias(?:es)?\s+(?:for|of)|\bsecond name for)\s+`/([A-Za-z0-9][\w:.-]*)`", re.IGNORECASE
)


def alias_target(description: str) -> str | None:
    """The command a row is an alias of when its description says "Alias for `/x`"."""
    m = REVERSE_ALIAS.search(description)
    return m.group(1) if m else None


def aliases_in(description: str, owner: str) -> list[str]:
    """Commands a description names as aliases of the row's command ("`/new` is an alias")."""
    if REVERSE_ALIAS.search(description) or not re.search(r"\balias", description, re.IGNORECASE):
        return []
    return [n for n in slash_names(description) if n != owner]


class Collector:
    """Names and aliases an extractor accumulates; aliases never repeat a name's owner."""

    def __init__(self):
        self.names: list[str] = []
        self.aliases: dict[str, list[str]] = {}

    def add(self, name: str, aliases=()):
        if name not in self.names:
            self.names.append(name)
        for a in aliases:
            if a != name and a not in self.aliases.setdefault(name, []):
                self.aliases[name].append(a)

    def result(self) -> dict:
        if not self.names:
            raise ValueError("no names found")
        return {"names": list(self.names), "aliases": {k: v for k, v in self.aliases.items() if v}}
