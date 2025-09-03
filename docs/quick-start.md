# Quick Start Guide

Get up and running with `ai-rulez` in minutes. This guide will walk you through the recommended workflow for creating, customizing, and generating a powerful configuration for your AI assistants.

---

### Step 1: Initialize Your Project

First, navigate to your project's root directory and run the `init` command. This creates a new `ai_rulez.yaml` file, pre-filled with best-practice rules and configurations for popular AI tools.

```bash
# Initialize a new configuration for your project
ai-rulez init "My Awesome Project" --popular
```

!!! tip "Choosing Providers"

    You can initialize for a specific tool using the `--preset` flag (e.g., `--preset cursor`) or for individual tools (e.g., `--claude --continue-dev`). The `--popular` flag is a great starting point.

### Step 2: Add Your Project's Context

Next, teach `ai-rulez` about your project using the `add` command. This is the most important step for getting high-quality, context-aware responses from your AI. The goal is to describe your tech stack, architecture, and workflow.

```bash
# Add a rule for your tech stack
ai-rulez add rule "Tech Stack" --priority critical --content "Frontend: React, TypeScript, Tailwind CSS. Backend: Go, PostgreSQL."

# Add a rule for your development workflow
ai-rulez add rule "Workflow" --priority high --content "All new features must have 90%+ unit test coverage."

# Add a documentation section for high-level context
ai-rulez add section "Project Goal" --content "This is a web service for managing customer invoices."
```

!!! info "Why use `add` instead of editing YAML?"

    Using the CLI ensures your configuration remains valid and consistent. It also makes it easy to manage your rules programmatically in scripts.

### Step 3: Generate Your Files

Now, run the `generate` command. `ai-rulez` will use your configuration to create tailored instruction files for all the providers you enabled during initialization.

```bash
ai-rulez generate
```

This will create files like `CLAUDE.md`, `.cursor/rules/rules.mdc`, and `.github/copilot-instructions.md`, all perfectly in sync.

### Step 4: Validate Your Configuration

Any time you make a change, you can run the `validate` command to ensure your configuration is syntactically correct and logically consistent.

```bash
ai-rulez validate
```

!!! success "You're All Set!"

    That's it! You now have a powerful, maintainable, and synchronized configuration for all your AI tools. Your AI assistants will now have the context they need to provide much more accurate and relevant responses.

---

## What's Next?

- **[Best Practices](monorepo.md)**: Learn how to structure your configurations for large projects.
- **[Full CLI Reference](cli.md)**: Explore every command and flag for advanced management.
- **[Configuration Guide](configuration.md)**: Dive deeper into advanced topics like `extends` and `includes`.
