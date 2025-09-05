# AI-Rulez v2.0.0 Release Notes

## 🎉 Major Release: v2.0.0

AI-Rulez v2.0 is a major release that introduces powerful new features while streamlining the configuration format for better consistency and usability.

## 🚨 Breaking Changes

### 1. Template Format Structure
Templates now require an explicit object format with `type` and `value` fields:

```yaml
# v1 Format (deprecated)
template: "default"                    # Builtin template
template: "@path/to/template.tmpl"     # File template  
template: "{{.Rules}}"                  # Inline template

# v2 Format (required)
template:
  type: builtin
  value: default

template:
  type: file
  value: path/to/template.tmpl

template:
  type: inline
  value: "{{.Rules}}"
```

### 2. Priority System
Priorities changed from integers to semantic enums with automatic bucketing:

```yaml
# v1 Format
priority: 10    # or any integer

# v2 Format
priority: critical  # Options: critical, high, medium, low, minimal
```

The migration automatically converts your numeric priorities to appropriate enum values while preserving relative importance.

### 3. Field Naming Changes
- **Sections**: `title` → `name`
- **Outputs**: `file` → `path` (path is now the only way to specify outputs)

### 4. Removed Features
- **`user_rulez` section**: No longer supported. Use `.local` configuration files instead
- **`profile` field**: Removed from schema (was never implemented)
- **Legacy string templates**: All templates must use object format

## ✨ New Features

### 1. Presets System
A new simplified way to configure outputs for popular AI tools:

```yaml
# Instead of defining multiple outputs manually
presets:
  - popular     # Includes Claude, Cursor, Windsurf, and Copilot
  - gemini
  - continue-dev
```

Available presets: `popular`, `claude`, `cursor`, `copilot`, `gemini`, `windsurf`, `continue-dev`, `cline`, `amp`, `codex`

### 2. Model Context Protocol (MCP) Server
Launch an MCP server for direct AI tool integration:

```yaml
mcp_servers:
  - name: github
    command: npx
    args: ["-y", "@modelcontextprotocol/server-github"]
    env:
      GITHUB_TOKEN: "${GITHUB_TOKEN}"
```

Run with: `ai-rulez mcp start`

### 3. Custom Slash Commands
Define custom commands for AI assistants:

```yaml
commands:
  - name: newtask
    description: Start a new task with fresh context
    usage: "/newtask <description>"
    system_prompt: "Focus only on the new task description..."
```

### 4. Configuration Inheritance with `extends`
Inherit from a base configuration file:

```yaml
extends: ./base-config.yaml  # or https://example.com/shared-config.yaml
```

### 5. Agent Model Specification
Specify which model an agent should use:

```yaml
agents:
  - name: code-reviewer
    model: "claude-3-opus-20240229"  # or gpt-4, etc.
```

### 6. Named Targets Support
Reference predefined target patterns:

```yaml
rules:
  - name: "Documentation Rules"
    targets: ["@doc-files"]  # References a named target pattern
```

### 7. Auto-Generated IDs
Agents and sections now receive auto-generated IDs for stable references during merging and overrides.

### 8. Gitignore Management
Control whether generated files are added to .gitignore:

```yaml
gitignore: false  # Disable automatic .gitignore updates
```

## 🔧 Improvements

### CLI Enhancements
- Full CRUD operations for all configuration elements via CLI
- Recursive generation for monorepos: `ai-rulez generate --recursive`
- Better error messages with actionable hints
- Improved init command with preset support: `ai-rulez init "My Project" --preset popular`

### Performance
- Task binary caching prevents macOS GitHub API rate limits in CI
- Optimized test execution with better parallelization
- Streamlined build processes

### Code Quality
- Complete codebase refactoring (40-50% code reduction)
- 100% linting compliance achieved
- All cyclomatic complexity issues resolved
- Comprehensive test coverage across CLI, MCP, and integration suites

## 🐛 Bug Fixes
- Fixed priority display issues in generated outputs
- Resolved flaky remote URL tests
- Fixed named target resolution for Filter functions
- Corrected rule merging logic in extends/includes
- Fixed migration edge cases and test failures

## 🔄 Automatic Migration

AI-Rulez v2.0 includes automatic migration from v1 configurations:

```bash
ai-rulez generate
# ℹ️  Detected v1 configuration format, migrating to v2...
# ℹ️  Successfully migrated configuration from v1 to v2 format
```

Your original file is backed up before migration. If migration fails:
1. Check for `user_rulez` sections (no longer supported)
2. Verify `includes` are still valid
3. Run `ai-rulez validate` to identify issues

## 📝 Documentation
- Comprehensive migration guide at https://goldziher.github.io/ai-rulez/migration-guide/
- Updated configuration reference with all v2.0 features
- New quick-start guide with preset examples
- Full CLI reference documentation

## 📦 Dependencies
- Requires Go 1.21+ (CI updated to Go 1.25)
- All package managers supported: Go, npm, pip, Homebrew

## 🚀 Quick Upgrade

```bash
# Backup your configuration
cp ai-rulez.yml ai-rulez.v1.backup.yml

# Upgrade ai-rulez
brew upgrade ai-rulez  # or npm/pip/go install

# Generate with automatic migration
ai-rulez generate

# Validate the migration
ai-rulez validate
```

## 📊 Release Statistics
- **40+ commits** since v1.6.1
- **100+ files** changed
- **~2000 lines** removed through refactoring
- **All test suites** passing

---

**Full Changelog**: [v1.6.1...v2.0.0](https://github.com/Goldziher/ai-rulez/compare/v1.6.1...v2.0.0)

**Installation**: See [Quick Start Guide](https://goldziher.github.io/ai-rulez/quick-start/)

**Support**: [GitHub Issues](https://github.com/Goldziher/ai-rulez/issues)