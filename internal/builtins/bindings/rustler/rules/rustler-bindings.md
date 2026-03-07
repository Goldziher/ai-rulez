---
priority: high
---
# Rustler NIF Conventions
- Use `#[rustler::nif]` for Elixir NIF functions.
- Map Rust types to Elixir terms using Rustler encoders/decoders.
- Use `rustler::Error` for NIF error handling. Return tagged tuples `{:ok, value}` / `{:error, reason}`.
- Keep NIF functions fast to avoid scheduler issues. Use dirty schedulers for long operations.
- Test from Elixir using ExUnit.
