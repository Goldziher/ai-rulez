# Quick Start

Let's get you set up with `ai-rulez`. This takes about 5 minutes and will sync all your AI tools automatically.

---

### Step 1: Create your config

Navigate to your project root and run the `init` command. We recommend using the AI-powered configuration for the best experience:

```bash
# AI-powered initialization (recommended)
ai-rulez init "My Project" --preset popular --use-agent claude

# Or simple initialization without AI assistance
ai-rulez init "My Project" --preset popular
```

The **AI-powered approach** will:
- Analyze your codebase structure and tech stack
- Generate project-specific rules and documentation  
- Create specialized agents for your workflow
- Set up comprehensive configuration tailored to your project

If you don't have Claude installed or prefer a simpler setup, the basic command still works great and you can always add content later using the CLI.

**Basic initialization** creates:
```yaml
$schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v2.schema.json
metadata:
  name: "My Project"
presets:
  - "popular"
```

**AI-powered initialization** creates a much richer configuration with rules, sections, and agents based on your actual codebase. For example, a Go project might get:
```yaml
$schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v2.schema.json
metadata:
  name: "My Project"
  description: "Go microservice with REST API"
presets:
  - "popular"
rules:
  - name: "Go Standards"
    priority: high
    content: "Follow standard Go layout (cmd/, internal/, pkg/)"
  - name: "Error Handling"  
    priority: critical
    content: "Always handle errors properly, use wrapped errors"
sections:
  - name: "Architecture"
    content: "Clean architecture with dependency injection..."
agents:
  - name: "go-expert"
    description: "Go specialist for backend development"
```

### Step 2: Add your project details (if needed)

If you used the AI-powered initialization, your configuration already includes comprehensive project-specific rules and documentation. You can skip to Step 3!

If you used the basic initialization, add some context about your project:

```bash
# Add a rule for your tech stack
ai-rulez add rule "Tech Stack" --priority critical --content "Frontend: React, TypeScript, Tailwind CSS. Backend: Go, PostgreSQL."

# Add a rule for your development workflow
ai-rulez add rule "Workflow" --priority high --content "All new features must have 90%+ unit test coverage."
```

**Pro tip**: You can also re-run the init command with AI assistance to enhance an existing configuration:
```bash
# Enhance existing config with AI analysis
ai-rulez init "My Project" --preset popular --use-agent claude --yes
```

Using the CLI keeps your config valid and makes it easy to script changes later.

### Step 3: Generate files

Run the `generate` command to create all your AI instruction files.

```bash
ai-rulez generate
```

This reads your `ai-rulez.yml` file and creates all the necessary configuration files for the tools included in the "popular" preset.

!!! info "Automatic .gitignore Updates"
    Generated files are automatically added to `.gitignore` to keep your repository clean. These files are regenerated from your config, so they don't need to be committed. To disable this, use `--update-gitignore=false` or set `gitignore: false` in your config.

### Step 4: Validate (optional)

Check that everything looks good:

```bash
ai-rulez validate
```

!!! success "You're All Set!"

    That's it! You now have a powerful, maintainable, and synchronized configuration for all your AI tools. Your AI assistants will now have the context they need to provide much more accurate and relevant responses.

---

## What's Next?

- **[Configuration Guide](configuration.md)**: Learn about advanced features like custom `outputs`, `extends`, and `includes`.
- **[Best Practices](monorepo.md)**: Learn how to structure your configurations for large projects.
- **[Full CLI Reference](cli.md)**: Explore every command and flag for advanced management.
