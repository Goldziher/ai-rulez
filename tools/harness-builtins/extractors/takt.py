"""TAKT: the bundled facets (instructions, knowledge, personas) at the pinned commit."""

FACETS = "builtins/en/facets/"
KINDS = {"command": "instructions", "skill": "knowledge", "agent": "personas"}


def facet_names(paths: list[str], directory: str) -> list[str]:
    """Names of the markdown files directly under builtins/en/facets/<directory>/.

    A facet is referenced by its file name without the extension; a subdirectory
    (such as instructions/_system/) holds files that are not referenced by name.
    """
    prefix = f"{FACETS}{directory}/"
    names = []
    for path in paths:
        rest = path[len(prefix) :] if path.startswith(prefix) else ""
        if rest.endswith(".md") and "/" not in rest:
            names.append(rest[: -len(".md")])
    if not names:
        raise ValueError(f"no facets under {prefix}")
    return names


def extract(ctx) -> dict:
    paths = ctx.tree(FACETS)
    return {kind: {"names": facet_names(paths, directory)} for kind, directory in KINDS.items()}
