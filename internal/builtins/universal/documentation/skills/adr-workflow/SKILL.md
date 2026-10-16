---
name: adr-workflow
description: "Write, update or supersede an Architecture Decision Record (MADR). Load when a change picks one design among credible alternatives, sets a rule tooling or reviewers enforce, reverses an earlier ADR, or when asked to document a decision."
---

## When to write one

A design picked among alternatives that others must live with, a rule that tooling enforces, or a reversal. Not for bug fixes, routine dependency bumps or a choice one PR fully explains.

## Steps

- Copy the template.
- Fill the mandatory sections: Context and Problem Statement, Considered Options, Decision Outcome.
- Name the file, command or PR behind each fact. Mark what you could not verify as "not verified".
- Keep it to one decision.
- Add the index row in the same PR.

## Superseding

Check whether the ADR is on the default branch. If not, edit it in place. If it is, write a new ADR and change only the old one's status and date.

## Statuses

`proposed`, `accepted`, `rejected`, `deprecated`, `superseded by ADR-<area>/NNNN`.

## Optional extension

A "Measured impact" section for decisions with numbers: before, after, how measured, tool version.
