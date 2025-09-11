# Welcome to ai-rulez

Ever notice how your `CLAUDE.md` says one thing, and your team's Copilot setup is completely different?

You're not alone. Managing AI context across multiple tools is messy. `ai-rulez` fixes this with one simple approach: write your AI instructions once, generate the rest.

### One File, Multiple Outputs

Write your AI context in a single `ai-rulez.yml` file. Generate instruction files for Claude, Cursor, OpenCode, and other formats automatically.

### CLI for Everything

Add rules, update agents, generate files—all from the command line. No more hunting through YAML files.

### MCP Integration

Let your AI assistant manage the configuration directly through the built-in MCP server. Perfect for automated workflows.

### Team Collaboration

Share configurations across your team with `extends` and `includes`. Everyone stays in sync.

---

## How it works

`ai-rulez` is a CLI tool that takes one `ai-rulez.yml` file and generates native configuration files for all your AI tools. 

Think of it as a build system for AI context—you write the source once, and it compiles to whatever format each tool needs.

## Next Steps

- **[Quick Start Guide](quick-start.md)**: Get up and running in minutes.
- **[Migration Guide](migration-guide.md)**: Upgrading from v1.x to v2.0.
- **[Best Practices](monorepo.md)**: Learn how to structure your configurations for large projects.
- **[Full CLI Reference](cli.md)**: Explore every command and flag.
