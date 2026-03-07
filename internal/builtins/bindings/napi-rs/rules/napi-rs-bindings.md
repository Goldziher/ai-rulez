---
priority: high
---
# NAPI-RS Binding Conventions
- Use `#[napi]` macro for Node.js-visible functions and classes.
- Map Rust errors to JavaScript `Error` objects using `napi::Error`.
- Use `napi::bindgen_prelude::*` for common type conversions.
- Support both CommonJS and ESM output. Generate `.d.ts` type definitions.
- Test bindings from JavaScript/TypeScript using `vitest` or `jest`.
