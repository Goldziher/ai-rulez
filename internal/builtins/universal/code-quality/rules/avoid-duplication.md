---
priority: medium
---
Extract shared logic after the third repetition, not before. Premature abstraction creates worse coupling than duplication. When extracting, ensure the shared code has a single reason to change — if two callers would evolve the logic differently, keep them separate.
