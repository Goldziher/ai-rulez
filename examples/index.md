# AI-Rulez Examples

This directory contains comprehensive examples demonstrating various features and configurations of ai-rulez.

## 📁 Example Categories

### 🎯 Presets
**Location:** [`presets/`](./presets/)

Tool-specific preset configurations that are automatically generated when using `--preset` flag:
- Claude configuration with agents
- Cursor IDE integration
- Windsurf editor rules
- GitHub Copilot instructions
- And more...

### 🔧 Full Configuration
**Location:** [`full-configuration/`](./full-configuration/)

Complete example showing all supported AI tools and configuration options:
- All supported output formats
- Multiple AI agents
- Comprehensive rules and sections
- Every supported tool integration

### 📝 Basic Examples

#### Simple Configuration
**File:** [`simple.yaml`](./simple.yaml)

Minimal configuration with basic rules - perfect for getting started.

#### With Agents
**File:** [`with-agents.yaml`](./with-agents.yaml)

Demonstrates AI agent configuration for Claude's specialized sub-assistants.

#### With Sections
**File:** [`with-sections-clean.yaml`](./with-sections-clean.yaml)

Shows how to organize complex instructions using sections.

### 🎨 Advanced Features

#### Target-Based Filtering
**File:** [`with-targets.yaml`](./with-targets.yaml), [`with-named-targets.yaml`](./with-named-targets.yaml)

Demonstrates how to filter rules for specific outputs using target patterns.

#### Remote Includes
**File:** [`remote-includes.yaml`](./remote-includes.yaml)

Shows how to include rules from remote URLs or local files.

#### Enterprise Setup
**File:** [`enterprise-setup.yaml`](./enterprise-setup.yaml)

Large-scale configuration for enterprise projects with multiple teams.

#### Full Featured
**File:** [`full-featured.yaml`](./full-featured.yaml)

Comprehensive example using all major features together.

## 🚀 Quick Start

1. **Basic setup:**
   ```bash
   ai-rulez init "MyProject"
   ```

2. **With a preset:**
   ```bash
   ai-rulez init "MyProject" --preset claude
   ```

3. **With AI agent assistance:**
   ```bash
   ai-rulez init "MyProject" --use-agent claude
   ```

4. **Multiple tools:**
   ```bash
   ai-rulez init "MyProject" --claude --cursor --copilot
   ```

## 📖 Example Usage

### Generate from an example:

```bash
# Copy an example
cp examples/simple.yaml ai_rulez.yaml

# Generate output files
ai-rulez generate

# Validate configuration
ai-rulez validate
```

### Try different configurations:

```bash
# Test with agents
ai-rulez generate examples/with-agents.yaml

# Test enterprise setup
ai-rulez generate examples/enterprise-setup.yaml
```

## 🔍 Understanding the Examples

Each example demonstrates specific features:

| Example | Key Features | Use Case |
|---------|-------------|----------|
| `simple.yaml` | Basic rules | Quick start, simple projects |
| `with-agents.yaml` | AI agents | Claude with specialized assistants |
| `full-configuration/` | All tools | Reference implementation |
| `presets/` | Tool-specific | Pre-configured for specific tools |
| `with-targets.yaml` | Targeting | Different rules for different outputs |
| `remote-includes.yaml` | Shared rules | Multi-project standards |
| `enterprise-setup.yaml` | Scale | Large teams and projects |

## 📚 Learn More

- [Main Documentation](../README.md)
- [CLI Commands Reference](../docs/commands.md)
- [Schema Documentation](https://github.com/Goldziher/ai-rulez/schema/)
- [Contributing Guide](../CONTRIBUTING.md)

## 💡 Tips

1. **Start simple:** Begin with `simple.yaml` and add features as needed
2. **Use presets:** Leverage presets for quick tool-specific setups
3. **Validate often:** Run `ai-rulez validate` after making changes
4. **Version control:** Commit your `ai_rulez.yaml` to track changes
5. **Share rules:** Use remote includes for team-wide standards