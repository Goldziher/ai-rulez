---
name: napi-rs-bindings
description: napi-rs conventions for exposing a Rust core to Node.js: #[napi] macros, napi::Error mapping, auto-generated .d.ts types, async Promises, and CommonJS/ESM output. Load when generating or reviewing napi-rs Node.js bindings for a Rust library.
---

- Use `#[napi]` macro for Node.js-visible functions and classes. Use `#[napi(constructor)]` for constructors.
- Map Rust errors to JavaScript `Error` objects via `napi::Error`. Return `Result<T>` from all fallible functions.
- Use `napi::bindgen_prelude::*` for common type conversions. Use `Buffer` for binary data.
- Support both CommonJS and ESM output. Generate `.d.ts` type definitions automatically.
- Use `#[napi(ts_return_type = "...")]` for complex TypeScript types the macro can't infer.
- Build with `napi build --release`. Test from TypeScript/JavaScript using a JS test runner (e.g., Vitest or Jest).
- Async: use `#[napi]` on `async fn` for Promise-returning functions. Use `AsyncTask` for CPU-bound work.
- Keep Node.js wrappers thin — business logic in Rust, JS provides idiomatic API.
- Handle `BigInt`, `Date`, and other JS-specific types explicitly.
- Anti-patterns: no panics in napi functions, no blocking the event loop, no manual `JsValue` manipulation.
