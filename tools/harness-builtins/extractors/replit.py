"""Replit Agent: the pre-defined skills of the skills directory page (markdown table)."""

from hb import doctables

PAGE = "https://docs.replit.com/features/agent/skills-directory.md"


def parse_skills(text: str) -> list[str]:
    """Names in the second column of the "Replit skills" table, which lists them comma-separated by
    category. They are the titles the skill picker shows."""
    found = doctables.find_tables(doctables.md_tables(text), "Replit skills", ["Category", "Skills"])
    if len(found) != 1:
        raise ValueError(f"expected one Category/Skills table under 'Replit skills', found {len(found)}")
    names = [n.strip() for row in found[0]["rows"] for n in row[1].split(",") if n.strip()]
    if not names:
        raise ValueError("no skills found")
    return names


def extract(ctx) -> dict:
    return {"skill": {"names": parse_skills(ctx.get(PAGE))}}
