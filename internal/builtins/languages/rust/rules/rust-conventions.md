---
priority: high
---
# Rust Conventions
- Use Rust 2021+ edition idioms. Prefer `impl Trait` over `dyn Trait` where possible.
- Handle errors with `Result<T, E>` and the `?` operator. Avoid `.unwrap()` in library code.
- Use `clippy` with pedantic lints. Format with `rustfmt`.
- Prefer zero-cost abstractions. Minimize `unsafe` blocks and document safety invariants.
- Use `cargo test`, `cargo clippy`, and `cargo fmt --check` in CI.
