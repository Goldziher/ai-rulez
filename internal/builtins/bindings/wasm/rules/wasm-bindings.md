---
priority: high
---

- Use `wasm-bindgen` for JavaScript interop. Use `wasm-pack` for building and packaging.
- Map Rust types to JS with `#[wasm_bindgen]`. Use `JsValue` for dynamic types, typed wrappers for known types.
- Handle errors with `Result<T, JsValue>`. Convert Rust errors to descriptive JS error messages.
- Support both browser (ES modules) and Node.js (CommonJS/ESM) targets.
- Async: use `wasm-bindgen-futures` for Promise-returning functions. Never block the main thread.
- Bundle size: use `opt-level = "z"`, `lto = true`, `codegen-units = 1` in release profile.
- Use `web-sys` and `js-sys` for Web API access. Feature-gate platform-specific APIs.
- Test with `wasm-pack test --headless --firefox` and a JS test runner (e.g., Vitest or Jest) for JS integration tests.
- Generate TypeScript definitions automatically. Keep `.d.ts` files in sync with Rust exports.
- Anti-patterns: no `std::thread` (single-threaded), no synchronous I/O, no panics (becomes JS exception).
