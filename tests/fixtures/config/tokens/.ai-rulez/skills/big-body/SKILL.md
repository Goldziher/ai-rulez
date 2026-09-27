---
description: "Short."
priority: medium
---

# Big Body

This skill exists to prove that a report splits a skill's description from its
body. The description above is two words. The body below is deliberately many
times larger, because that asymmetry is the whole reason a per-file token total
misleads: the description is the part a harness may put in front of the model
before the skill is ever opened, and the body is the part it only pays for once
the model decides to read it.

## Step one

Read the configuration and resolve every include, then walk the content tree so
that rules, context files, skills, agents and commands are all in memory before
anything is rendered.

## Step two

Render each provider's outputs from that in-memory tree. Never read a generated
file back off disk to measure it: a stale or partially written tree produces a
number that is wrong in a way nobody will notice.

## Step three

Report the result split by when an agent loads it, and say plainly which parts
of the cost the tool cannot see at all.
