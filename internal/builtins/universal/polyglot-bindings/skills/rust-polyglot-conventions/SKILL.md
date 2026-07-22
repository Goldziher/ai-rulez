---
name: rust-polyglot-conventions
description: Rust conventions for a core that backs multiple language bindings: stable-edition tooling, typed Result errors with no panics in library paths, rustdoc with error docs, isolated unsafe, Send futures, FFI-friendly DTOs, and single-source version syncing. Load when writing or reviewing a Rust core exposed through polyglot bindings.
---

- Use the current stable Rust edition, `cargo fmt`, and `clippy -D warnings` for Rust core and binding crates.
- Library code returns `Result<T, E>` with typed errors. Do not `unwrap`, panic, or exit in library paths.
- Public APIs have rustdoc, error documentation, and examples that use `?` instead of `unwrap`.
- Unsafe code has `SAFETY` comments and should be isolated behind small, tested abstractions.
- Async public futures are `Send` where host runtimes may move them across threads.
- Public DTOs that cross binding boundaries are FFI-friendly or have explicit binding-safe equivalents.
- Keep binding-safe DTOs separate from integration-specific request and response types.
- Avoid Rust-only API shapes that cannot be represented naturally in target languages.
- Use one source of truth for version numbers and sync binding manifests from it during release.
