<!-- Excerpt of https://kiro.dev/docs/reference/slash-commands.md, fetched 2026-10-18 -->

## Using slash commands

Only available in interactive chat mode.

## Available commands

### `/help`

Switch to the Help Agent to ask questions about Kiro CLI features, or display classic help text.

```bash
# Switch to Help Agent
/help

# Ask a question directly
/help How do I configure MCP servers?

# Show classic help text
/help --legacy

# Show help for a specific command
/help --legacy /context
```

See [Help Agent](https://kiro.dev/docs/custom-agents/built-in.md) for more details.


### `/quit`

Exit the interactive chat session.

```bash
/quit
```

Aliases: /exit, /q


### `/agent`

Manage agents and switch between different agent configurations.

```bash
# List all available agents
/agent list

# Create a new agent (AI-assisted by default)
/agent create my-agent

# Create with description and MCP servers
/agent create my-agent -D "Code reviewer" -m code-analysis

# Create using editor-based mode instead of AI generation
/agent create my-agent --manual

# Create in a specific directory
/agent create my-agent --directory workspace

# Generate is an alias for create
/agent generate my-agent

# Edit the current agent
/agent edit

# Edit a specific agent by name (interactive picker when no name given)
/agent edit my-agent

# Edit an agent by file path
/agent edit --path /absolute/path/to/my-agent.json

# Show agent config schema
/agent schema

# Set default agent (interactive picker when no name given)
/agent set-default my-agent

# Swap to a different agent at runtime
/agent swap code-reviewer
```

**Available subcommands:**

- `list` - List all available agents with their descriptions
- `create` - Create a new agent using AI-assisted generation by default. Pass `--manual` for editor-based creation. Supports `-D` (description), `-m` (MCP server), `-d` (directory), and `-f` (template agent) flags.
- `edit` - Edit an existing agent configuration. Shows an interactive picker when no agent name is provided. Supports `--path` for direct file access.
- `generate` - Alias for `create`
- `schema` - Show agent config schema
- `set-default` - Define a default agent for new chat sessions. Shows an interactive picker when no agent name is provided.
- `swap` - Swap to a new agent at runtime (shows agent descriptions for selection)

**Notes:**

- Agent creation and editing are available in a local V3 session started with `kiro-cli --v3`. Cloud sessions don't support these operations.
- `/agent create` uses AI-assisted generation by default. Pass `--manual` to create the profile in your configured editor.
- `/agent edit` opens the selected profile in your configured editor.
- Agents can be stored globally in `~/.kiro/agents/` or per-workspace in `.kiro/agents/`
- The `--directory` flag accepts `workspace`, `global`, or a custom path
- Launch kiro-cli chat with a specific agent using `kiro-cli chat --agent agent_name`
- Built-in agents (`kiro_default`, `kiro_help`, `kiro_planner`) cannot be edited
- Type `/agent` or `/agent swap` and press Tab to autocomplete agent names. Ghost text hints appear inline as you type, matching the behavior in `/model`.
- Switching agents preserves your current `/model` selection unless the target agent's configuration specifies a model of its own — then it switches to that one.

When you edit an agent, changes are made in a **temporary file** first. If validation fails (for example, invalid JSON or missing required fields), you're prompted to continue editing or cancel. The original configuration file is never overwritten by a failed edit.

See [Custom Agents](https://kiro.dev/docs/custom-agents.md) for detailed documentation.


### `/mcp`

View MCP servers and registry status.

```bash
/mcp
```

Display Model Context Protocol servers currently active, their available tools, and registry status for managed servers.

In the MCP panel, pressing `Enter` starts OAuth authentication for the selected server only when it is enabled, configured, and has a pending OAuth URL.

When your organization has a registry configured, `/mcp add` and `/mcp remove` persist changes to your agent's configuration file, so additions and removals survive restarts.

**MCP disabled by your administrator**

If your organization's administrator has disabled MCP in the Kiro console, `/mcp` shows one of two messages:

- **`MCP has been disabled by your administrator`** — MCP is turned off for your organization. All servers (user-configured, legacy, registry, and session-injected) are suppressed and no MCP tools are available to the agent.
- **`Failed to retrieve MCP settings — MCP disabled`** — the Kiro client couldn't reach the governance API. MCP fails closed for security. This is usually transient; retry, check connectivity, or contact your administrator if it persists.

MCP governance applies to IAM Identity Center and API key users. Builder ID and social auth users are not affected.

See [MCP governance](https://kiro.dev/docs/enterprise/governance/mcp.md) for the administrator setup.

**Related commands:**

- `/mcp auth` — Force re-authentication for a remote server
- `/mcp cancel-auth` — Cancel a stuck authentication flow
- `/mcp logout` — Remove stored credentials for a server

### `/mcp auth`

Force re-authentication for a remote MCP server whose OAuth token has expired or become invalid.

```bash
# Re-authenticate with a specific server
/mcp auth my-server
```

Use this when a remote MCP server stops responding due to an expired or revoked token. Kiro opens a browser-based OAuth flow to obtain a fresh token without requiring you to restart your session.

You can also press `^A` in the MCP panel status view to trigger the same action.


### `/changelog`

View recent release notes inline in the CLI.

```bash
/changelog
```

Displays the latest Kiro CLI release notes directly in your terminal, so you can see what changed without leaving your session.

### Skill-based slash commands

In addition to the built-in commands above, skills defined in `.kiro/skills/` and `~/.kiro/skills/` are automatically available as slash commands. Type `/` followed by the skill name to invoke it directly.

```bash
# Invoke a skill named "pr-review"
/pr-review

# Invoke a skill named "cdk-deploy"
/cdk-deploy
```

The available skill commands depend on which skills are present in your workspace and global skill directories. See [Agent Skills](https://kiro.dev/docs/skills.md) for details on creating and managing skills.

### Keyboard shortcuts

In interactive mode, you can also use:

- `Ctrl+C` - Cancel current input / exit session
- `Ctrl+D` - Exit session
-
