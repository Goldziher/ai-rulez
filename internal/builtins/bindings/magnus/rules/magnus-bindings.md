---
priority: high
---
# Magnus Binding Conventions
- Use `magnus` crate for Ruby native extensions.
- Map Rust types to Ruby using `magnus::Value` and type conversion traits.
- Use `magnus::Error` for Ruby exception handling.
- Build with `rb_sys` and `rake-compiler`. Support multiple Ruby versions.
- Test bindings from Ruby using RSpec or Minitest.
