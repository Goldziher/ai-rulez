# AI-Powered Rule Enforcement

AI-Rulez provides **real-time rule enforcement** using AI agents to automatically detect violations and apply fixes across your codebase. This powerful feature extends beyond simple static analysis by leveraging the contextual understanding of AI models to interpret and enforce complex coding standards.

## Overview

The enforcement system analyzes your code against the rules and sections defined in your `ai-rulez.yaml` configuration, using AI agents to:

- 🔍 **Detect violations** with contextual understanding
- 🛠️ **Suggest and apply fixes** automatically
- 📊 **Generate comprehensive reports** in multiple formats
- 🔄 **Provide iterative review workflows** for continuous improvement
- 🎯 **Scale enforcement** across large codebases

## Quick Start

```bash
# Basic enforcement (read-only by default)
ai-rulez enforce

# Automatically apply fixes
ai-rulez enforce --fix

# Use specific AI agent
ai-rulez enforce --agent gemini --fix
```

## Supported AI Agents

AI-Rulez integrates with all major AI coding assistants:

| Agent | Command | Description |
|-------|---------|-------------|
| **Claude** | `claude` | Anthropic's AI assistant with excellent code analysis |
| **Gemini** | `gemini` | Google's AI model with strong violation detection |
| **Cursor** | `cursor` | AI-powered code editor integration |
| **AMP** | `amp` | Sourcegraph's AI assistant |
| **Codex** | `codex` | OpenAI's code model |
| **Continue.dev** | `continue-dev` | Open-source coding assistant |

The system automatically detects which agents are available on your system and gracefully handles missing tools.

## Enforcement Levels

Configure enforcement strictness based on your workflow:

- **`warn`** (default): Log violations but don't fail the process
- **`error`**: Fail on violations but don't apply fixes automatically
- **`fix`**: Automatically apply suggested fixes when possible
- **`strict`**: Fail immediately on any violation, suitable for CI/CD

## Basic Usage

### Read-Only Mode (Default)

By default, enforcement runs in read-only mode, detecting and reporting violations without making changes:

```bash
# Analyze entire codebase
ai-rulez enforce

# Use specific agent
ai-rulez enforce --agent claude

# Check with error level
ai-rulez enforce --level error
```

### Fix Mode

Enable automatic fix application with the `--fix` flag:

```bash
# Apply fixes automatically
ai-rulez enforce --fix

# Fix with specific enforcement level
ai-rulez enforce --fix --level strict

# Use specific agent for fixes
ai-rulez enforce --fix --agent gemini
```

### File and Rule Filtering

Target specific files or rules:

```bash
# Include specific file patterns
ai-rulez enforce --include-files "src/**/*.js,tests/**/*.ts"

# Exclude file patterns
ai-rulez enforce --exclude-files "vendor/**,*.min.js"

# Enforce only specific rules
ai-rulez enforce --only-rules "no-console-output,proper-error-handling"

# Exclude specific rules
ai-rulez enforce --exclude-rules "documentation-required"
```

## Output Formats

Generate reports in various formats for different use cases:

### JSON Output
```bash
ai-rulez enforce --format json --output violations.json
```

Perfect for automation and CI/CD integration.

### CSV Export
```bash
ai-rulez enforce --format csv --output report.csv
```

Ideal for spreadsheet analysis and tracking over time.

### Summary Format
```bash
ai-rulez enforce --format summary
```

Provides a concise overview of enforcement results.

### Table Format (Default)
```bash
ai-rulez enforce --format table
```

Human-readable tabular output with violation details.

## Review Workflow

The review system provides iterative code improvement through AI-powered feedback loops:

### Basic Review
```bash
# Enable review with default settings
ai-rulez enforce --review

# Set quality threshold (0-100)
ai-rulez enforce --review --review-threshold 85

# Configure review iterations
ai-rulez enforce --review --review-iterations 5
```

### Advanced Review Options
```bash
# Multi-agent review (different agents for enforcement vs review)
ai-rulez enforce --agent gemini --review --review-agent claude

# Auto-approve after reaching threshold
ai-rulez enforce --review --review-auto-approve

# Require improvement between iterations
ai-rulez enforce --review --require-improvement

# Custom review timeout
ai-rulez enforce --review --review-timeout 45s
```

### Review Process

The review workflow:

1. **Initial Analysis**: Primary agent analyzes code and suggests fixes
2. **Review Phase**: Review agent evaluates the quality of suggestions
3. **Quality Scoring**: Each iteration receives a score (0-100%)
4. **Iterative Improvement**: Multiple rounds refine the analysis
5. **Approval Decision**: Based on threshold and improvement requirements

The AI reviewer evaluates:
- ✅ Code quality and adherence to rules
- ✅ Appropriateness of suggested fixes
- ✅ Overall improvement between iterations
- ✅ Compliance with project standards

## Git Hook Integration

### Lefthook Configuration

Add to your `.lefthook.yml`:

```yaml
pre-commit:
  parallel: true
  commands:
    ai-rulez-enforce:
      run: ai-rulez enforce --level error --agent gemini
      fail_text: "AI-Rulez enforcement failed - fix violations before committing"
      stage_fixed: true
```

### Pre-commit Hooks

Add to your `.pre-commit-config.yaml`:

```yaml
repos:
  - repo: https://github.com/Goldziher/ai-rulez
    rev: v2.4.3
    hooks:
      - id: ai-rulez-enforce
      - id: ai-rulez-enforce-fix
```

The shared hooks execute `scripts/pre-commit/run-ai-rulez.sh`, which downloads the appropriate prebuilt binary if `ai-rulez` is not already on your `PATH`. You can point to an existing binary with `AI_RULEZ_BINARY` or pin a different release using `AI_RULEZ_VERSION`.

### GitHub Actions Integration

```yaml
name: AI-Rulez Enforcement
on: [push, pull_request]

jobs:
  enforce:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Install AI-Rulez
        run: go install github.com/Goldziher/ai-rulez/cmd@latest
      - name: Enforce Rules
        run: ai-rulez enforce --level error --agent gemini --format json --output violations.json
      - name: Upload Results
        uses: actions/upload-artifact@v3
        if: always()
        with:
          name: violations
          path: violations.json
```

## Configuration Examples

### Project-Specific Configuration

```yaml
# ai-rulez.yaml
version: "1.0"
name: "My Project"

rules:
  - name: "no-console-output"
    description: "Prohibit console.log and print statements in production code"
    enforcement:
      level: error
      agent: "gemini"

  - name: "proper-error-handling"
    description: "All errors must be properly handled"
    enforcement:
      level: strict
      max_violations: 0

  - name: "documentation-required"
    description: "Functions must have documentation"
    enforcement:
      level: warn
      agent: "claude"

sections:
  - name: "code-quality"
    description: "General code quality standards"
    enforcement:
      level: error
      review: true
      review_threshold: 80
    rules:
      - "Use meaningful variable names"
      - "Functions should be under 50 lines"
      - "Avoid deeply nested code (max 3 levels)"
```

### Language-Specific Enforcement

```bash
# JavaScript/TypeScript projects
ai-rulez enforce --include-files "**/*.{js,ts,jsx,tsx}" --agent cursor

# Python projects
ai-rulez enforce --include-files "**/*.py" --agent claude

# Go projects
ai-rulez enforce --include-files "**/*.go" --agent gemini
```

## Advanced Features

### Timeout Management
```bash
# Custom timeout per file
ai-rulez enforce --timeout 45s

# Review-specific timeout
ai-rulez enforce --review --review-timeout 60s
```

### Violation Limits
```bash
# Maximum violations before stopping
ai-rulez enforce --max-violations 10

# Unlimited violations
ai-rulez enforce --max-violations -1
```

### Verbose Logging
```bash
# Detailed execution logs
ai-rulez enforce --verbose

# Debug-level information
ai-rulez enforce --debug
```

## Troubleshooting

### Common Issues

**Agent Not Found**
```bash
# Check available agents
which claude gemini cursor

# Install missing agents or use available ones
ai-rulez enforce --agent gemini
```

**Timeout Issues**
```bash
# Increase timeout for large files
ai-rulez enforce --timeout 60s

# Reduce file scope
ai-rulez enforce --include-files "src/**/*.js" --exclude-files "**/*.min.js"
```

**Performance Optimization**
```bash
# Process specific file patterns only
ai-rulez enforce --include-files "**/*.{js,py,go}"

# Exclude large directories
ai-rulez enforce --exclude-files "node_modules/**,vendor/**"
```

### Agent-Specific Notes

- **Gemini**: Excellent violation detection, may require longer timeouts
- **Claude**: Strong at contextual analysis, good for complex rules
- **Cursor**: Fast execution, good for simple violations
- **AMP**: May need specific prompt formatting for optimal results

## Best Practices

1. **Start with Read-Only**: Test enforcement without `--fix` first
2. **Use Appropriate Levels**: Reserve `strict` for critical rules
3. **Agent Selection**: Different agents excel at different violation types
4. **File Filtering**: Target relevant files to improve performance
5. **Review Workflows**: Use for complex code quality improvements
6. **CI/CD Integration**: Implement in development workflow, not just CI
7. **Team Consistency**: Document agent choices and enforcement levels

## Performance Considerations

- **File Scope**: Limit to relevant files for faster execution
- **Agent Selection**: Some agents are faster than others
- **Parallel Processing**: Future versions will support parallel execution
- **Caching**: AI responses may be cached for repeated analyses
- **Timeout Configuration**: Balance thoroughness with speed

The AI-powered enforcement system transforms static rule checking into intelligent, context-aware code quality assurance that scales with your team and project complexity.
