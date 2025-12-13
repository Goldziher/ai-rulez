# V3 Migration Test Fixtures

This directory contains test fixtures for V2 to V3 migration testing.

## Fixtures

### basic/
Simple V2 configuration with basic rules and sections. Used to test:
- Basic migration workflow
- Rule conversion to .ai-rulez/rules/{name}.md
- Section conversion to .ai-rulez/context/{name}.md
- Single preset (claude)

### with-agents/
V2 configuration with agents. Used to test:
- Agent conversion to .ai-rulez/skills/{id}/SKILL.md
- Multiple presets (claude, cursor)
- Agent metadata preservation

### multi-preset/
V2 configuration using the "popular" preset. Used to test:
- Preset expansion (popular → claude, cursor, gemini, windsurf, copilot)
- Target preservation across multiple outputs

### complex/
Complex V2 configuration with all content types. Used to test:
- Multiple rules, sections, and agents
- Priority preservation
- Target lists
- Multiple presets
- Complete end-to-end migration

## Usage

These fixtures are used by the tests in `internal/migration/v3_migrate_test.go` to validate the migration logic.
