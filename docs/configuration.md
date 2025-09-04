# Configuration (`ai_rulez.yaml`)

This page provides a complete reference for the `ai_rulez.yaml` file format, with explanations for key concepts.

## Top-Level Properties

These are the main keys at the root of your configuration file.

### `metadata`

An object containing essential information about your project.

```yaml
metadata:
  name: "My Project"
  version: "1.0.0"
  description: "A brief description of the project."
```

### `extends`

A string specifying a single base configuration file to inherit from. The path can be a local file or a remote URL.

!!! info "Why use `extends`?"
    The `extends` property is for **inheritance**. It allows you to create a foundational configuration (e.g., for your entire organization) and have individual projects inherit and build upon it. This is the best way to enforce common standards while allowing for project-specific customization.

### `includes`

A list of strings specifying other `ai_rulez.yaml` files to merge into your configuration. Paths can be local files or remote URLs.

!!! info "Why use `includes`?"
    The `includes` property is for **composition**. It allows you to create modular, reusable sets of rules (e.g., for a specific language, framework, or security standard) and mix them into any project as needed.

### `outputs`

A list of objects defining the files to be generated. See the `output` object reference below for details.

### `rules`

A list of rule objects that define instructions for your AI assistant. See the `rule` object reference below for details.

### `sections`

A list of section objects that define large blocks of documentation or context. See the `section` object reference below for details.

### `agents`

A list of agent objects that define specialized sub-assistants. See the `agent` object reference below for details.

### `mcp_servers`

A list of MCP server objects for integrating external tools. See the `mcp_server` object reference below for details.

### `commands`

A list of custom slash command objects for your AI assistant. See the `command` object reference below for details.

---

## Object Reference

This section details the fields for each type of object in the configuration.

### `output` Object

Defines a file or directory to be generated.

- `path` (string, required): The output path. If it ends with a `/`, it is treated as a directory.
- `type` (string): For directory outputs, specifies the content type to generate (`rule` or `agent`).
- `naming_scheme` (string): For directory outputs, a pattern for filenames (e.g., `{name}.md`).
- `template` (object): A structured object to define the template.
  - `type` (string, required): `builtin`, `file`, or `inline`.
  - `value` (string, required): The name, path, or content of the template.

### `rule` Object

- `name` (string, required): The name of the rule.
- `content` (string, required): The content of the rule.
- `id` (string): An optional unique identifier.
- `priority` (string): `critical`, `high`, `medium`, `low`, or `minimal`.
- `targets` (array of strings): A list of output paths this rule applies to.

!!! tip "When to use an `id`"
    The `id` field is used to create a stable identifier for a rule that is independent of its `name`. This is useful when you want to override a rule from an included or extended configuration file without relying on the `name`, which might change.

### `section` Object

- `name` (string, required): The name of the section.
- `content` (string, required): The content of the section.
- `id` (string): An optional unique identifier for overriding.
- `priority` (string): `critical`, `high`, `medium`, `low`, or `minimal`.
- `targets` (array of strings): A list of output paths this section applies to.

### `agent` Object

- `name` (string, required): The name of the agent.
- `description` (string, required): A description of the agent's purpose.
- `id` (string): An optional unique identifier for overriding.
- `priority` (string): The agent's priority.
- `tools` (array of strings): A list of tools available to the agent.
- `model` (string): An optional model identifier (e.g., `claude-3-opus-20240229`, `gpt-4`).
- `system_prompt` (string): The system prompt for the agent.
- `template` (object): A structured template object for the system prompt.
- `targets` (array of strings): A list of output paths this agent applies to.

!!! info "The Power of Agents"
    Agents allow you to create specialized "experts" for different tasks. For example, you can create a `database-expert` with a system prompt full of details about your schema and query patterns. This provides the AI with deep, focused context, resulting in much more accurate and helpful responses for domain-specific questions.

### `mcp_server` Object

- `name` (string, required): The name of the server.
- `id` (string): An optional unique identifier for overriding.
- `description` (string): A description of the server.
- `command` (string): The command to start the server.
- `args` (array of strings): Arguments for the command.
- `env` (object): Environment variables for the server.
- `transport` (string): `stdio`, `sse`, or `http`.
- `url` (string): The URL for remote servers.
- `enabled` (boolean): Whether the server is enabled.
- `targets` (array of strings): A list of output paths this server applies to.

### `command` Object

- `name` (string, required): The name of the command (e.g., `new-task`).
- `description` (string, required): A description of the command.
- `id` (string): An optional unique identifier for overriding.
- `aliases` (array of strings): Alternative names for the command.
- `usage` (string): A usage example (e.g., `/new-task <description>`).
- `system_prompt` (string): The system prompt to use for this command.
- `shortcut` (string): A keyboard shortcut.
- `enabled` (boolean): Whether the command is enabled.
- `targets` (array of strings): A list of output paths this command applies to.
