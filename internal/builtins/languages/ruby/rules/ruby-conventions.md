---
priority: high
---

- Ruby 3.2+, `frozen_string_literal: true`, `.ruby-version` file.
- Linting: `rubocop` with auto-fix (120 char max). Plugins: `rubocop-rspec`, `rubocop-performance`.
- Type checking: RBS + `steep check`. Use `rbs prototype` for scaffolding type signatures.
- Testing: RSpec with `describe`/`context`/`it`, `factory_bot` over fixtures, `simplecov` (80%+).
- Security: `brakeman` for SAST (Rails), `bundler-audit` for dependency CVE scanning.
- Error handling: specific exceptions inheriting `StandardError`, no bare `rescue`.
- Composition over inheritance, `Comparable`/`Enumerable` mixins, `case/in` pattern matching.
- Dependencies: `bundler`, commit `Gemfile.lock`, pessimistic `~>` constraints.
- Gem packaging: use `gemspec` with `bundler` gem template, `rake release` for distribution.
- `&:method_name` block shorthand, `=>` pattern matching destructuring (3.2+).
- Anti-patterns: monkey patching, `method_missing` without `respond_to_missing?`, `eval` with user input.
