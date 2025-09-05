# Quick Start

Let's get you set up with `ai-rulez`. This takes about 5 minutes and will sync all your AI tools automatically.

---

### Step 1: Create your config

Navigate to your project root and run the `init` command with a preset. This creates a simple `ai-rulez.yml` file using the recommended `presets` feature.

```bash
# Use the "popular" preset for Claude, Cursor, Windsurf, and Copilot
ai-rulez init "My Project" --preset popular
```

This will generate the following `ai-rulez.yml` file:

```yaml
$schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v2.schema.json
metadata:
  name: "My Project"
presets:
  - "popular"
```

The `init` command can also analyze your codebase to suggest rules and agents, but for now, we'll stick with the basics.

### Step 2: Add your project details

Now, add some context about your project. This helps your AI understand your tech stack and workflow. You can add rules, sections, and more directly from the command line.

```bash
# Add a rule for your tech stack
ai-rulez add rule "Tech Stack" --priority critical --content "Frontend: React, TypeScript, Tailwind CSS. Backend: Go, PostgreSQL."

# Add a rule for your development workflow
ai-rulez add rule "Workflow" --priority high --content "All new features must have 90%+ unit test coverage."
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
