# CRUD Operations Examples

This guide demonstrates how to use ai-rulez CLI commands to manage your configuration.

## Initial Setup

Start with a basic configuration:

```bash
# Initialize a new project
ai-rulez init my-project

# This creates a basic ai-rulez.yaml file
```

## Adding Rules

```bash
# Add a new rule interactively
ai-rulez add rule "API Security"

# You'll be prompted for:
# - Priority (1-10)
# - Content (multiline, end with Ctrl+D)

# Example input:
# Priority: 10
# Content:
# Always validate user input
# Use parameterized queries
# Implement rate limiting
# ^D

# Add a rule with ID for precise matching
ai-rulez add rule "Performance" --id "perf-001"
```

## Adding Sections

```bash
# Add a documentation section
ai-rulez add section "Getting Started"

# You'll be prompted for:
# - Priority (optional)
# - Content (multiline)

# Example input:
# Priority: 100
# Content:
# # Getting Started Guide
# 
# This project uses AI assistants for development.
# Follow these steps to get started...
# ^D
```

## Adding Outputs

```bash
# Add a new output file
ai-rulez add output "docs/guidelines.md"

# You'll be prompted for template (optional)
# Template: documentation

# Add a directory output for agents
ai-rulez add output ".claude/agents/" --template "default"
```

## Updating Existing Items

```bash
# Update a rule's content
ai-rulez update rule "API Security"
# Enter new content when prompted

# Update a rule's priority
ai-rulez update rule "Performance" --priority 9

# Update a section
ai-rulez update section "Getting Started"
# Enter new content when prompted

# Update an output's template
ai-rulez update output "docs/guidelines.md" --template "@templates/custom.tmpl"
```

## Deleting Items

```bash
# Delete a rule
ai-rulez delete rule "Performance"

# Delete by ID for precise targeting
ai-rulez delete rule --id "perf-001"

# Delete a section
ai-rulez delete section "Getting Started"

# Delete an output
ai-rulez delete output "docs/guidelines.md"
```

## Validation

```bash
# Validate your configuration
ai-rulez validate

# Validate a specific file
ai-rulez validate ./configs/team-rules.yaml

# Validate recursively
ai-rulez validate -r
```

## Generation

```bash
# Generate all outputs
ai-rulez generate

# Preview changes without writing
ai-rulez generate --dry-run

# Generate and update .gitignore
ai-rulez generate --update-gitignore

# Generate recursively for multiple configs
ai-rulez generate -r

# Generate a specific config
ai-rulez generate ./configs/api-rules.yaml
```

## Git Hooks Setup

```bash
# Initialize with automatic hook setup
ai-rulez init --setup-hooks

# This will detect and configure:
# - Lefthook (if lefthook.yaml exists)
# - Pre-commit (if .pre-commit-config.yaml exists)
# - Husky (if .husky/ directory exists)
```

## Working with MCP Server

```bash
# Start MCP server for AI assistant integration
ai-rulez mcp

# The server exposes these tools:
# - get_rules: Retrieve rules with filtering
# - get_sections: Get documentation sections
# - generate_output: Generate files
# - validate_config: Validate configuration
# - add_rule/section/output: Add new items
# - update_rule/section/output: Update items
# - delete_rule/section/output: Remove items

# Configure Claude Desktop to use it:
# Add to Claude's config.json:
{
  "ai-rulez": {
    "command": "ai-rulez",
    "args": ["mcp"]
  }
}
```

## Batch Operations Script

```bash
#!/bin/bash
# batch-update.sh - Example of scripting ai-rulez

# Add multiple rules from a file
while IFS='|' read -r name priority content; do
  echo "$content" | ai-rulez add rule "$name" --priority "$priority"
done < rules.txt

# Update all configs in a directory
for config in configs/*.yaml; do
  ai-rulez validate "$config"
  ai-rulez generate "$config"
done

# Export rules to JSON for processing
ai-rulez validate --format json > rules.json
```

## Common Workflows

### Adding a New Team Member's Preferences

```bash
# They create their own local file
cat > ai-rulez.local.yaml << EOF
userRulez:
  - name: "Personal Style"
    content: "Use 2 spaces, not tabs"
EOF

# Generate with their preferences
ai-rulez generate
```

### Updating Organization Standards

```bash
# Pull latest standards
curl -o company-standards.yaml https://standards.company.com/ai-rules.yaml

# Validate compatibility
ai-rulez validate

# Generate updated configs
ai-rulez generate --update-gitignore

# Commit changes
git add -A
git commit -m "chore: update to latest company standards"
```

### Debugging Configuration Issues

```bash
# Validate with verbose output
ai-rulez validate -v

# Test generation without writing
ai-rulez generate --dry-run

# Check specific rule loading
ai-rulez get rules --name "API Security"

# Check configuration hierarchy
ai-rulez validate --show-hierarchy
```