---
priority: high
---
# WASM Binding Conventions
- Use `wasm-bindgen` for JavaScript interop. Use `wasm-pack` for building and packaging.
- Map Rust types to JS using `#[wasm_bindgen]`. Support both browser and Node.js environments.
- Handle errors with `JsValue` / `JsError`. Minimize data copying across the WASM boundary.
- Use `web-sys` and `js-sys` for Web API access. Keep WASM binary size minimal.
- Test with `wasm-pack test` targeting browser and Node.js.
