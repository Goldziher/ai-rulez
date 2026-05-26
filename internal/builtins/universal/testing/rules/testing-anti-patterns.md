---
priority: high
---

Do not test mock behavior instead of real behavior. Do not add test-only methods to production code. Do not mock what you don't own — wrap it and test the wrapper. Do not test implementation details — test observable behavior. Do not write tests that pass when the code is broken. If a test never fails, it's not testing anything.
