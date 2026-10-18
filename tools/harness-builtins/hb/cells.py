"""Read command names and aliases out of a documentation table cell or sentence."""

import re

NAME = r"[A-Za-z0-9][\w:.-]*"
CODE = re.compile(r"`([^`]+)`")
ALIAS_WORD = re.compile(r"\balias(?:es)?\b(?!\s+for\b)\s*:?\s*", re.IGNORECASE)
ALIAS_TOKEN = re.compile(r"`/?(" + NAME + r")`|(?<![\w`])/(" + NAME + r")")


def slash_token(token: str) -> str | None:
    """The command name of an inline-code token such as `/model <name>`, else None."""
    m = re.match(r"/(" + NAME + ")", token.strip())
    return m.group(1).rstrip(".") if m else None


def slash_names(text: str) -> list[str]:
    """Names of the backticked /commands in text, in order, without arguments."""
    return [n for n in (slash_token(t) for t in CODE.findall(text)) if n]


def split_alias_clause(text: str) -> tuple[str, list[str]]:
    """(text before the alias clause, alias names).

    The clause is the word alias or aliases (not "alias for") and the backticked or /-prefixed
    names after it, up to the closing parenthesis, the end of the sentence or the end of the text.
    """
    m = ALIAS_WORD.search(text)
    if not m:
        return text, []
    tail = text[m.end() :]
    end = re.search(r"\)|\.(?:\s|$)|<|;|\n", tail)
    clause = tail[: end.start()] if end else tail
    names = [a or b for a, b in ALIAS_TOKEN.findall(clause)]
    return text[: m.start()], [n.rstrip(".") for n in names]
