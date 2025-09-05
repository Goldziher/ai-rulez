# Quick Start

Let's get you set up with `ai-rulez`. This takes about 5 minutes and will sync all your AI tools automatically.

---

### Step 1: Create your config

Navigate to your project root and run the `init` command. This creates an `ai-rulez.yml` file with sensible defaults.

```bash
ai-rulez init "My Project" --popular
```

The `--popular` flag sets up Claude, Cursor, Windsurf, and Copilot. You can also use `--preset cursor` for just Cursor, or see `ai-rulez init --help` for all options.

The init command analyzes your codebase and creates a smart template with only required fields uncommented. If you have Claude, Continue.dev, Gemini, or AMP installed, it can even customize the template intelligently for your specific project.

### Step 2: Add your project details

Now add some context about your project. This helps your AI understand your tech stack and workflow.

```bash
# Add a rule for your tech stack
ai-rulez add rule "Tech Stack" --priority critical --content "Frontend: React, TypeScript, Tailwind CSS. Backend: Go, PostgreSQL."

# Add a rule for your development workflow
ai-rulez add rule "Workflow" --priority high --content "All new features must have 90%+ unit test coverage."

# Add a documentation section for high-level context
ai-rulez add section "Project Goal" --content "This is a web service for managing customer invoices."
```

Using the CLI keeps your config valid and makes it easy to script changes later.

### Step 3: Generate files

Run the `generate` command to create all your AI instruction files.

```bash
ai-rulez generate
```

This creates files like `CLAUDE.md`, `.cursor/rules/rules.mdc`, and `.github/copilot-instructions.md`—all automatically synced.

### Step 4: Validate (optional)

Check that everything looks good:

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
