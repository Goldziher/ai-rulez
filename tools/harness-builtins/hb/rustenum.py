"""Read the command names of a Rust enum that derives strum's EnumString.

strum names a variant by its kebab-cased identifier unless #[strum(to_string = "..")] or
#[strum(serialize = "..")] says otherwise. The first of to_string and serialize is the name,
the others are aliases.
"""

import re


def strip_comments(text: str) -> str:
    text = re.sub(r"/\*.*?\*/", "", text, flags=re.DOTALL)
    return re.sub(r"(?m)//.*$", "", text)


def kebab(ident: str) -> str:
    return re.sub(r"(?<=[a-z0-9])(?=[A-Z])", "-", ident).lower()


def enum_body(text: str, name: str) -> str:
    m = re.search(r"\benum\s+" + re.escape(name) + r"\s*\{", text)
    if not m:
        raise ValueError(f"enum {name} not found")
    depth, i = 1, m.end()
    while depth and i < len(text):
        depth += {"{": 1, "}": -1}.get(text[i], 0)
        i += 1
    return text[m.end() : i - 1]


def split_top(body: str) -> list[str]:
    parts, depth, cur = [], 0, []
    for ch in body:
        if ch in "([{":
            depth += 1
        elif ch in ")]}":
            depth -= 1
        if ch == "," and depth == 0:
            parts.append("".join(cur))
            cur = []
        else:
            cur.append(ch)
    parts.append("".join(cur))
    return [p.strip() for p in parts if p.strip()]


def variants(text: str, name: str) -> list[dict]:
    """[{variant, names: [primary, *aliases]}] for every variant that strum does not disable."""
    out = []
    for part in split_top(strip_comments(enum_body(text, name))):
        attrs = " ".join(re.findall(r"#\[strum\((.*?)\)\]", part, flags=re.DOTALL))
        if re.search(r"\bdisabled\b", attrs):
            continue
        ident = re.match(r"[A-Z][A-Za-z0-9]*", re.sub(r"#\[[^\]]*\]", "", part).strip())
        if not ident:
            raise ValueError(f"cannot read a variant from {part!r}")
        to_string = re.findall(r'to_string\s*=\s*"([^"]+)"', attrs)
        serialize = re.findall(r'serialize\s*=\s*"([^"]+)"', attrs)
        names = list(dict.fromkeys(to_string + serialize)) or [kebab(ident.group(0))]
        out.append({"variant": ident.group(0), "names": names})
    return out


def commands(text: str, name: str) -> dict:
    """{"names": [...], "aliases": {primary: [aliases]}} of the enum."""
    names, aliases = [], {}
    for v in variants(text, name):
        names.append(v["names"][0])
        if len(v["names"]) > 1:
            aliases[v["names"][0]] = v["names"][1:]
    return {"names": names, "aliases": aliases}
