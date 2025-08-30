# AI Agent Integration for Init Command - Implementation Tasks

## Overview
Enhance the `init` command to detect and use AI CLI tools (claude, amp, codex, cursor, gemini, aider) to generate ai-rulez configurations. Add support for presets to simplify common configuration patterns.

## Core Implementation Tasks

### 1. Presets System
- [ ] Define preset types (minimal, standard, full, enterprise)
- [ ] Create preset to outputs mapping structure
- [ ] Add `--preset` flag to init command (minimal, standard, full, enterprise)
- [ ] Implement preset resolution logic
- [ ] Add preset validation
- [ ] Unit test preset to outputs conversion
- [ ] Document available presets in help text

### 2. Agent Detection System
- [ ] Create `AgentInfo` struct with ID, command, display name
- [ ] Implement `detectAvailableAgents()` function using `exec.LookPath`
- [ ] Sort detected agents alphabetically
- [ ] Add `--list-agents` flag to show available agents
- [ ] Unit test agent detection with mocked exec.LookPath
- [ ] Handle agent command variations (e.g., cursor vs cursor-agent)

### 3. CLI Flags for Non-Interactive Mode
- [ ] Add `--use-agent` flag (accepts comma-separated list, case-insensitive)
- [ ] Add `--no-agent` flag to skip agent usage
- [ ] Implement flag validation and parsing
- [ ] Handle conflicts between interactive and non-interactive modes
- [ ] Unit test flag parsing and validation

### 4. Output Configuration Updates
- [ ] Add support for `AGENTS.md` (shared by AMP, Codex, Cursor)
- [ ] Add support for `.aider.conf.yml` generation
- [ ] Update config templates to include new tool outputs
- [ ] Implement provider grouping for shared files
- [ ] Add `providers` field to output configuration
- [ ] Unit test output configuration generation

### 5. Agent Prompt Templates
- [ ] Create base prompt template structure
- [ ] Write claude-specific prompt template
- [ ] Write amp-specific prompt template
- [ ] Write codex-specific prompt template
- [ ] Write cursor-specific prompt template
- [ ] Write gemini-specific prompt template
- [ ] Include YAML schema in prompts
- [ ] Add validation criteria to prompts
- [ ] Unit test prompt generation

### 6. Agent Invocation System
- [ ] Implement `invokeAgent(agentID, prompt)` function
- [ ] Add timeout handling (30 seconds default)
- [ ] Capture and parse agent output
- [ ] Handle agent errors gracefully
- [ ] Implement retry mechanism (max 3 attempts)
- [ ] Add progress indicators during agent invocation
- [ ] Mock agent invocation for unit tests

### 7. Validation and Feedback Loop
- [ ] Create YAML validation function
- [ ] Validate required fields (metadata, outputs, rules)
- [ ] Generate specific feedback for invalid configs
- [ ] Implement retry with feedback
- [ ] Add fallback to default template on failure
- [ ] Unit test validation logic
- [ ] Unit test feedback generation

### 8. Special Handling for Aider
- [ ] Skip Aider for config generation (read-only agent)
- [ ] Generate `.aider.conf.yml` with read list
- [ ] Include all generated rule files in read list
- [ ] Unit test Aider config generation
- [ ] Document Aider limitations

### 9. Integration Tests (Environment-Gated)
- [ ] Create `AI_RULEZ_INTEGRATION_TEST` environment variable check
- [ ] Write integration test for Claude
- [ ] Write integration test for AMP
- [ ] Write integration test for Codex
- [ ] Write integration test for Cursor
- [ ] Write integration test for Gemini
- [ ] Test retry mechanism with real agents
- [ ] Test timeout handling
- [ ] Add integration test documentation

### 10. Unit Tests
- [ ] Test agent detection logic
- [ ] Test preset resolution
- [ ] Test flag parsing and validation
- [ ] Test prompt template generation
- [ ] Test YAML validation
- [ ] Test output configuration generation
- [ ] Test error handling paths
- [ ] Test interactive vs non-interactive modes
- [ ] Achieve >80% code coverage

### 11. Linting and Code Quality
- [ ] Run `go fmt` on all new code
- [ ] Run `golangci-lint` and fix issues
- [ ] Add appropriate error wrapping
- [ ] Ensure consistent error messages
- [ ] Add proper logging statements
- [ ] Review and optimize performance

### 12. Documentation Updates
- [ ] Update README with new agent support
- [ ] Document all supported AI tools
- [ ] Add examples for each agent
- [ ] Document preset options
- [ ] Update help text for init command
- [ ] Add troubleshooting guide
- [ ] Create integration test running guide

### 13. Example Configurations
- [ ] Create example for Claude with agents
- [ ] Create example for AMP/Codex/Cursor shared AGENTS.md
- [ ] Create example for Aider integration
- [ ] Create example for each preset
- [ ] Add examples to documentation

## Testing Strategy

### Local Testing Checklist
- [ ] Test with Claude installed
- [ ] Test with AMP installed
- [ ] Test with Codex installed
- [ ] Test with Cursor installed
- [ ] Test with Gemini installed
- [ ] Test with Aider installed
- [ ] Test with no agents installed
- [ ] Test with multiple agents installed
- [ ] Test all presets
- [ ] Test interactive mode
- [ ] Test non-interactive mode
- [ ] Test retry mechanism
- [ ] Test timeout handling

### CI Testing
- [ ] Ensure integration tests are skipped by default
- [ ] Unit tests pass without any agents installed
- [ ] Linting passes
- [ ] Code coverage meets threshold

## Preset Definitions

### Minimal Preset
- Claude only (CLAUDE.md)
- No agents, no sections

### Standard Preset (Default)
- Claude with agents
- AGENTS.md (for AMP, Codex, Cursor)

### Full Preset
- All supported tools
- Claude with agents
- AGENTS.md
- GEMINI.md
- Cursor rules
- Windsurf (.windsurf/)
- Includes sections

### Enterprise Preset
- Full preset plus:
- GitHub Copilot (.github/copilot-instructions.md)
- Continue.dev (.continuerules)
- Cline (.clinerules/)

## Success Criteria
- [ ] All unit tests pass
- [ ] Integration tests pass when run locally
- [ ] Linting passes with no errors
- [ ] Code coverage >80%
- [ ] All supported agents work when installed
- [ ] Graceful handling when agents not installed
- [ ] Clear user feedback throughout process
- [ ] Documentation is complete and accurate