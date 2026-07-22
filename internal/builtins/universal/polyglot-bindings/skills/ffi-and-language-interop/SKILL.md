---
name: ffi-and-language-interop
description: FFI and native interop rules: single documented pointer ownership, opaque handles, null-pointer checks, allocate/free pairs, unsafe-block invariants, generated C headers, stable C ABI, frozen struct layouts, and error/context conversion. Load when writing or reviewing FFI boundaries between native and host languages.
---

- Every pointer has one owner, documented with `SAFETY` comments or host-language ownership docs.
- Use opaque handles across native boundaries. Do not expose Rust, C++, or host-runtime internals directly.
- Check all nullable pointers before use and return explicit errors for invalid handles or null inputs.
- Provide allocate/free pairs for every caller-owned allocation and document which side frees each value.
- Every unsafe block must state the invariant, why it holds, and what would make it invalid.
- Generate C headers or native declarations from source when possible, and make CI verify generated files are current.
- Treat the C ABI or equivalent native ABI as the stable contract for downstream bindings.
- Freeze native struct layouts across compatible releases, and version breaking ABI changes deliberately.
- Convert native errors to host exceptions, result objects, or idiomatic error values at every boundary.
- Preserve diagnostic context across boundaries: message, numeric code when available, source operation, and cause chain.
