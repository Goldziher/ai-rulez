"""Cline: slash commands of the CLI TUI, of the VS Code webview and of the SDK runtime at the pinned tag."""

import re

TUI = "apps/cli/src/tui/commands/slash-command-registry.ts"
WEBVIEW = "apps/vscode/webview-ui/src/components/chat/chat-view/hooks/useMessageHandlers.ts"
BUILTIN = "apps/vscode/src/sdk/builtin-slash-commands.ts"


def strip_comments(text: str) -> str:
    text = re.sub(r"/\*.*?\*/", "", text, flags=re.DOTALL)
    return re.sub(r"(?m)^\s*//.*$", "", text)


def array_literal(text: str, declaration: str) -> str:
    """The text between the brackets of `<declaration> ... = [ ... ]`."""
    m = re.search(re.escape(declaration) + r"[^=]*=\s*\[", text)
    if not m:
        raise ValueError(f"{declaration} not found")
    depth, i = 1, m.end()
    while depth and i < len(text):
        depth += {"[": 1, "]": -1}.get(text[i], 0)
        i += 1
    if depth:
        raise ValueError(f"{declaration}: unbalanced brackets")
    return text[m.end() : i - 1]


def entry_names(text: str, declaration: str) -> list[str]:
    """The `name: "x"` fields of the entries of an array declaration."""
    body = array_literal(strip_comments(text), declaration)
    names = re.findall(r'\bname:\s*"([^"]+)"', body)
    if not names:
        raise ValueError(f"{declaration} has no entries")
    return names


def compaction_commands(text: str) -> list[str]:
    """The commands the webview runs as a compaction instead of sending them to the model."""
    m = re.search(r"function isCompactionCommand\([^)]*\)[^{]*\{(.*?)\n\}", strip_comments(text), flags=re.DOTALL)
    names = re.findall(r'===\s*"/([a-z][a-z0-9-]*)"', m.group(1)) if m else []
    if not names:
        raise ValueError("isCompactionCommand not found")
    return names


def extract(ctx) -> dict:
    return {
        "command_tui": {"names": entry_names(ctx.raw(TUI), "const TUI_LOCAL_COMMANDS")},
        "command_webview": {"names": compaction_commands(ctx.raw(WEBVIEW))},
        "command_builtin": {"names": entry_names(ctx.raw(BUILTIN), "export const BUILTIN_SLASH_COMMANDS")},
    }
