---
priority: high
---

- Ruby 3.2+, `frozen_string_literal: true`, `.ruby-version` file.
- Linting: a linter/formatter (e.g., RuboCop) with auto-fix (120 char max), plus its RSpec/performance plugins.
- Type checking: RBS with a type checker (e.g., steep). Use `rbs prototype` for scaffolding type signatures.
- Testing: RSpec (or Minitest) with `describe`/`context`/`it`, factories over fixtures, a coverage tool (e.g., SimpleCov) at 80%+.
- Security: a SAST tool (e.g., Brakeman for Rails), `bundler-audit` for dependency CVE scanning.
- Error handling: specific exceptions inheriting `StandardError`, no bare `rescue`.
- Composition over inheritance, `Comparable`/`Enumerable` mixins, `case/in` pattern matching.
- Dependencies: `bundler`, commit `Gemfile.lock`, pessimistic `~>` constraints.
- Gem packaging: use `gemspec` with `bundler` gem template, `rake release` for distribution.
- `&:method_name` block shorthand, `=>` pattern matching destructuring (3.2+).
- Anti-patterns: monkey patching, `method_missing` without `respond_to_missing?`, `eval` with user input.
