# ai-rulez v2.0.0 Breaking Changes Implementation

## Overview
This document tracks the implementation of breaking changes for ai-rulez v2.0.0. Each task should be marked as completed once implemented and tested.

## 🔴 Schema Breaking Changes

### 1. ✅ Implement local config file support (COMPLETED)
- [x] Remove `user_rules` section entirely from schema
- [x] Add local config file discovery (`.local` suffix support)  
- [x] Implement config merging logic with ID-based overrides
- [x] Update loader to use local config discovery
- [x] Update all code references
- **Implementation:**
  - Local configs: `ai-rulez.local.yaml`, `.ai-rulez.local.yaml`, etc.
  - ID-based overrides: items with matching `id` fields override main config
  - Name-based fallback: if no ID match, override by `name`
  - **Files modified:**
    - `schema/ai-rules-v2.schema.json`
    - `internal/config/types.go` 
    - `internal/config/finder.go`
    - `internal/config/merger.go` (new)
    - `internal/config/loader.go`

### 2. ✅ Unify output configuration (COMPLETED)
- [x] Remove `outputs.file` from schema
- [x] Update schema to use only `outputs.path`
- [x] Update `Output` struct in `internal/config/types.go`
- [x] Remove `GetPath()` method (no longer needed)
- [x] Update all code references throughout codebase
- **Implementation:**
  - Single `path` field for all outputs
  - Direct `.Path` access instead of method calls
  - **Files modified:**
    - `schema/ai-rules-v2.schema.json`
    - `internal/config/types.go`
    - All files with `output.GetPath()` calls (21 replacements)

### 3. ✅ Priority system enum (COMPLETED - Core Implementation)
- [x] Update schema to use enum: `critical | high | medium | low | minimal`
- [x] Add Priority type in Go with constants and conversion methods
- [x] Update Go structs to use Priority type
- [x] Update default priority handling
- [x] Add priority parsing and validation
- [ ] **TODO**: Complete CRUD command updates for all priority handling
- **Implementation:**
  - Priority mapping: critical=10, high=8, medium=5, low=3, minimal=1
  - **Files modified:**
    - `schema/ai-rules-v2.schema.json`
    - `internal/config/types.go` 
    - `internal/config/priority.go` (new)
    - `internal/config/loader_helpers.go`
    - `cmd/commands/crud/rules.go` (partial)

### 4. ✅ Remove global targets map (COMPLETED)
- [x] Remove `targets` field from root schema
- [x] Remove from `Config` struct  
- [x] Remove ResolveTargets function entirely
- [x] Update all code that used ResolveTargets
- [x] Verify item-level targets still work
- **Implementation:**
  - Direct glob patterns only (no @named-targets)
  - Validation prevents @ prefixed patterns
  - **Files modified:**
    - `internal/config/types.go`
    - `internal/config/targets.go`
    - `internal/config/validation.go` 
    - `internal/config/merger.go`
    - `internal/templates/template.go`

### 5. ✅ Restructure template configuration (COMPLETED)
- [x] Update schema to use structured template object
- [x] Add template type field: `builtin | file | inline`
- [x] Update template parsing logic  
- [x] Update all template references
- [x] Support both legacy string and new structured format
- **Implementation:**
  - New structured format with type/value fields
  - Backwards compatible with legacy string format
  - **New structure:**
    ```yaml
    template:
      type: builtin | file | inline
      value: "template_name" | "path/to/file" | "inline content"
    ```
  - **Files modified:**
    - `schema/ai-rules-v2.schema.json`
    - `internal/config/types.go`
    - `internal/config/template.go` (new)
    - `internal/generator/render.go`

## 🟡 CLI Enhancements

### 6. Add `get` command for single item retrieval
- [ ] Create `get` command structure
- [ ] Implement `get rule <name>`
- [ ] Implement `get section <name>`
- [ ] Implement `get agent <name>`
- [ ] Implement `get output <index>`
- [ ] Add tests
- **Files to create:**
  - `cmd/commands/crud/get.go`
  - `cmd/commands/crud/get_test.go`

### 7. Add `--targets` flag to CRUD commands
- [ ] Add targets flag to `add rule` command
- [ ] Add targets flag to `add section` command
- [ ] Add targets flag to `add agent` command
- [ ] Add targets flag to `update rule` command
- [ ] Add targets flag to `update section` command
- [ ] Add targets flag to `update agent` command
- [ ] Update flag parsing and validation
- **Files to modify:**
  - `cmd/commands/crud/rules.go`
  - `cmd/commands/crud/sections.go`
  - `cmd/commands/crud/agents.go`

### 8. Add `--dry-run` flag to generate command
- [ ] Add dry-run flag to generate command
- [ ] Implement preview logic without writing files
- [ ] Show diff of what would change
- [ ] Add tests
- **Files to modify:**
  - `cmd/commands/generate.go`
  - `internal/generator/generator.go`

### 9. Add `--format` flag to list commands
- [ ] Add format flag supporting: `table | json | yaml`
- [ ] Implement formatters for each type
- [ ] Update all list commands
- [ ] Add tests
- **Files to modify:**
  - `cmd/commands/crud/list.go`
  - Create `internal/output/formatter.go`

## 🟢 New Features

### 10. Add rule categories
- [ ] Add `category` field to schema
- [ ] Update Rule struct
- [ ] Add category to CRUD commands
- [ ] Update list command to group by category
- [ ] Add category filter to list command
- **Files to modify:**
  - `schema/ai-rules-v2.schema.json`
  - `internal/config/types.go`
  - `cmd/commands/crud/rules.go`

### 11. Add conditional rules
- [ ] Add `condition` field to schema
- [ ] Implement condition evaluation logic
- [ ] Support glob patterns for file existence checks
- [ ] Update generator to respect conditions
- **Files to modify:**
  - `schema/ai-rules-v2.schema.json`
  - `internal/config/types.go`
  - `internal/generator/generator.go`

### 12. Add configuration inheritance
- [ ] Add `extends` field to schema
- [ ] Implement config merging logic
- [ ] Handle circular dependencies
- [ ] Update loader to process extends
- **Files to modify:**
  - `schema/ai-rules-v2.schema.json`
  - `internal/config/config.go`
  - `internal/config/loader.go`

### 13. Add output type `mixed`
- [ ] Update schema to include `mixed` type
- [ ] Update generator to handle mixed outputs
- [ ] Create mixed output template
- **Files to modify:**
  - `schema/ai-rules-v2.schema.json`
  - `internal/generator/generator.go`
  - `internal/templates/templates.go`

### 14. Validate agent tools
- [ ] Define known tools enum in schema
- [ ] Add validation for tool names
- [ ] Update MCP handlers
- [ ] Add tool existence validation
- **Files to modify:**
  - `schema/ai-rules-v2.schema.json`
  - `internal/config/validation.go`
  - `internal/mcp/handlers/agents.go`

## 🔵 Quality of Life Improvements

### 15. Add bulk operations support
- [ ] Add `--from-file` flag to add commands
- [ ] Implement bulk import logic
- [ ] Add validation for bulk operations
- [ ] Add tests
- **Files to modify:**
  - `cmd/commands/crud/common.go`
  - All CRUD add commands

### 16. Add export command
- [ ] Create `export` command structure
- [ ] Implement `export rules`
- [ ] Implement `export sections`
- [ ] Implement `export agents`
- [ ] Support different output formats
- **Files to create:**
  - `cmd/commands/export.go`
  - `cmd/commands/export_test.go`

### 17. Add config templates to init
- [ ] Create template definitions
- [ ] Add `--template` flag to init command
- [ ] Create templates: `monorepo`, `library`, `app`
- [ ] Update init logic
- **Files to modify:**
  - `cmd/commands/init.go`
  - `internal/templates/init_templates.go`

### 18. Add validation to all update commands
- [ ] Add `--validate` flag to update commands
- [ ] Validate before saving
- [ ] Show validation errors clearly
- [ ] Add tests
- **Files to modify:**
  - `cmd/commands/crud/update.go`
  - All update command files

## 📋 Testing & Documentation

### 19. Update all test fixtures
- [ ] Update test YAML files to new schema
- [ ] Fix all breaking test cases
- [ ] Add tests for new features
- [ ] Update benchmark tests
- **Files to modify:**
  - All files in `testing/`
  - All `*_test.go` files

### 20. Update documentation
- [ ] Update README with v2.0 changes
- [ ] Update all example YAML files
- [ ] Update CLI documentation
- [ ] Create migration guide
- [ ] Update schema documentation
- **Files to modify:**
  - `README.md`
  - `docs/` directory
  - Example files

### 21. Update MCP handlers
- [ ] Update for all schema changes
- [ ] Test MCP tools with new schema
- [ ] Update MCP documentation
- **Files to modify:**
  - `internal/mcp/handlers/*.go`

## 📊 Implementation Status

**Total Tasks:** 21  
**Completed:** 5 (All major schema breaking changes ✅)  
**In Progress:** 0  
**Remaining:** 16

### 🎉 Phase 1 Complete - Major Schema Breaking Changes
All core schema restructuring is now complete! The v2.0 schema is significantly cleaner with:
- Local config override support
- Simplified output configuration  
- Human-readable priority system
- Clean target patterns (no global targets)
- Structured template configuration

---

## Implementation Order (Recommended)

1. **Phase 1: Schema Changes** (Tasks 1-5)
   - Start with schema changes as they affect everything else
   - Update Go types to match

2. **Phase 2: Core Features** (Tasks 10-14)
   - Add new fields and capabilities
   - Implement core logic changes

3. **Phase 3: CLI Enhancements** (Tasks 6-9)
   - Add new commands
   - Enhance existing commands

4. **Phase 4: Quality of Life** (Tasks 15-18)
   - Add convenience features
   - Improve user experience

5. **Phase 5: Testing & Docs** (Tasks 19-21)
   - Fix all tests
   - Update documentation
   - Final validation

---

## Notes

- Each task should be tested individually before marking as complete
- Run full test suite after each phase
- Update this document as implementation progresses
- Consider creating feature branches for each phase