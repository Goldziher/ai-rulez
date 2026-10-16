---
name: pytest-triage
description: "Diagnoses failing pytest runs from the output the user pastes or the last local run. Use when tests fail and the cause is unclear; not for writing new tests."
---

# Pytest triage

1. Run `pytest -x -q --tb=short` unless the user pasted output.
2. Find the first failing test and read the test and the code it calls.
3. Match the failure against `references/common-failures.md`.
4. Report the likely cause in two sentences and propose a fix. Do not edit files unless the user asks.
