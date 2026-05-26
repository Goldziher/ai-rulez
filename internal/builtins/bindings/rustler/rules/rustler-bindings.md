---
priority: high
---

- Use `#[rustler::nif]` for Elixir NIF functions. Use `#[module = "Elixir.ModuleName"]` for module naming.
- Return tagged tuples: `{:ok, value}` / `{:error, reason}`. Use Rustler encoders/decoders for type mapping.
- Use `rustler::Error` for NIF error handling. Map Rust errors to descriptive Elixir error tuples.
- Keep NIF functions fast (<1ms) to avoid scheduler issues. Use `#[rustler::nif(schedule = "DirtyCpu")]` for long operations.
- Build with `mix compile` (Rustler compiler). Test from Elixir using ExUnit.
- Use `ResourceArc<T>` for sharing Rust state across NIF calls. Implement `Drop` for cleanup.
- Use `#[derive(NifStruct)]` and `#[derive(NifUnitEnum)]` for Elixir-compatible types.
- Keep Elixir NIF wrapper thin — business logic in Rust, Elixir provides functional API.
- Handle binary data with `Binary` and `OwnedBinary` types.
- Anti-patterns: no panics in NIFs (crashes the BEAM VM), no blocking the scheduler, no I/O in NIFs.
