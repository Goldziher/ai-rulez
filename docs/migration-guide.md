# Migration Guide: v1.x to v2.0

## Automatic Migration

Migration happens automatically when you run any ai-rulez command on a v1 configuration. Your original file is backed up and restored if migration fails.

```bash
ai-rulez generate
# ℹ️  Detected v1 configuration format, migrating to v2...
# ℹ️  Successfully migrated configuration from v1 to v2 format
```

## What Changes

### Templates: String → Object Format
```yaml
# Before
template: "default"

# After  
template:
  type: builtin
  value: default
```

**All template types:**
- Builtin: `"default"` → `{type: "builtin", value: "default"}`
- File: `"@path/file.tmpl"` → `{type: "file", value: "path/file.tmpl"}`
- Inline: `"{{.Rules}}"` → `{type: "inline", value: "{{.Rules}}"}`

### Priorities: Integer → Enum with Relative Bucketing
Your integer priorities are converted to string enums while preserving their relative importance:

```yaml
# Before
rules:
  - name: High Priority Rule
    priority: 100
  - name: Medium Priority Rule  
    priority: 50
  - name: Low Priority Rule
    priority: 1

# After
rules:
  - name: High Priority Rule
    priority: critical      # highest in your config
  - name: Medium Priority Rule
    priority: medium        # middle in your config
  - name: Low Priority Rule
    priority: minimal       # lowest in your config
```

### Field Names
- **Outputs**: `file` → `path`
- **Sections**: `title` → `name`

### Auto-Generated IDs
Human-readable IDs are generated for agents and sections:
```yaml
# Before
agents:
  - name: code-reviewer
sections:
  - title: Project Overview

# After
agents:
  - name: code-reviewer
    id: code-reviewer        # auto-generated
sections:
  - name: Project Overview   # title → name
    id: project-overview     # auto-generated
```

## Migration Failures

### user_rulez (No Longer Supported)
```yaml
# This will fail migration
user_rulez:
  rules: [...]
```
**Solution**: Move content to `.local` configuration files.

### includes (Manual Migration Required)
```yaml  
# This will fail migration
includes:
  - shared/config.yaml
```
**Solution**: Manually merge included content or use the new `extends` field.

## Manual Migration

If automatic migration fails, update manually:

1. **Update schema URL**:
   ```yaml
   $schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v2.schema.json
   ```

2. **Convert templates** to object format (see examples above)

3. **Convert priorities** from integers to enums:
   - `critical`, `high`, `medium`, `low`, `minimal`

4. **Update field names** (`file`→`path`, `title`→`name`)

5. **Add IDs** for agents and sections

6. **Validate**: `ai-rulez validate`

## New v2.0 Features

### MCP Server Integration
```yaml
mcp_servers:
  - name: github
    command: npx
    args: ["-y", "@modelcontextprotocol/server-github"]
```

### Custom Commands
```yaml
commands:
  - name: newtask
    description: Start a new task
    system_prompt: "Focus on the new task only."
```

### Configuration Inheritance  
```yaml
extends: ./base-config.yaml
```

### Agent Models
```yaml
agents:
  - name: specialist
    model: "claude-3-opus-20240229"
```