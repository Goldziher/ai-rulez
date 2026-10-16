---
name: acme-skill-standards
description: "Acme rubric for skill files. Use when judging whether a skill meets the Acme skill standards."
---

# Acme skill standards

## C1 - Trigger in the description

The `description` frontmatter field contains the phrase "Use when" followed by a concrete situation.

## C2 - Verify section

The body has a `## Verify` heading that is followed by at least one command to run or one observable check.

## C3 - Non-goals section

The body has a `## Not for` heading that names at least one situation the skill does not cover.

## C4 - Owner line

The last non-empty line of the file starts with `Owner:` and names a team.

## C5 - Bounded length

The body is at most 80 lines long and contains no heading deeper than level 3.
