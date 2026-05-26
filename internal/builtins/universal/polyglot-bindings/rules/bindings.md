---
priority: critical
---

- Bindings are minimal glue: call the core library, convert types, convert errors — no business logic.
- Canonical surface: core API first, stable native ABI second, language bindings third.
- Keep integrations and framework adapters outside language bindings unless they are the binding's public purpose.
- Give each binding its own language-native test suite and run it in CI.
- Generated binding and e2e code must not be hand-edited. Change the source schema, generator, or core API instead.
- Error conversion must preserve context across every boundary: message, numeric code when available, and operation.
- Async boundaries must match host runtime conventions; do not block event loops, schedulers, or UI threads.
