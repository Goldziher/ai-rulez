# Configuration Reference

Everything you can put in your `ai-rulez.yml` file.

## Main Structure

The key sections in your config file.

### `metadata`

Basic info about your project.

```yaml
metadata:
  name: "My Project"
  version: "1.0.0"
  description: "A brief description of the project."
```

### `presets`

The recommended way to configure output files for common AI tools. This is a list of strings, where each string is a supported preset name.

```yaml
# Use presets for popular tools
presets:
  - "popular" # Includes Claude, Cursor, Windsurf, and Copilot
  - "gemini"
```

See the "Configuring Outputs" section below for a full list of available presets.

### `outputs`

The advanced method for configuring output files. This gives you fine-grained control over file paths, naming schemes, and templates. Use this when a preset doesn't meet your needs or you have a custom tool.

```yaml
# For advanced, custom configurations
outputs:
  - path: "CUSTOM_PROMPT.md"
    template:
      type: "inline"
      value: "My custom prompt template..."
```

### `extends`

A string specifying a single base configuration file to inherit from. The path can be a local file or a remote URL.

!!! info "Why use `extends`?"
    The `extends` property is for **inheritance**. It allows you to create a foundational configuration (e.g., for your entire organization) and have individual projects inherit and build upon it. This is the best way to enforce common standards while allowing for project-specific customization.

### `includes`

A list of strings specifying other `ai-rulez.yml` files to merge into your configuration. Paths can be local files or remote URLs.

!!! info "Why use `includes`?"
    The `includes` property is for **composition**. It allows you to create modular, reusable sets of rules (e.g., for a specific language, framework, or security standard) and mix them into any project as needed.

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

### `gitignore`

Controls whether `ai-rulez` automatically updates your `.gitignore` file with generated files.

```yaml
gitignore: false  # Disable automatic .gitignore updates
```

By default, `ai-rulez` will add all generated files to your `.gitignore` to prevent them from being committed to version control. This is usually what you want, since these files are regenerated from your configuration.

- **Default**: `true` (automatically update `.gitignore`)
- **Set to `false`**: If you want to manage `.gitignore` manually or commit generated files

!!! tip "CLI Override"
    You can override this setting using the `--update-gitignore` flag when running `ai-rulez generate`. The CLI flag takes precedence over the configuration setting.

---

## Configuring Outputs: Presets vs. Outputs

`ai-rulez` offers two ways to define the files it generates.

### Using Presets (Recommended)

For most users, `presets` are the simplest and recommended method. They are a shorthand for the most common AI tool configurations.

```yaml
presets:
  - "popular"
  - "gemini"
```

**Available Presets:**

-   `popular`: A convenient bundle that includes `claude`, `cursor`, `windsurf`, `copilot`, and `gemini`.
-   `claude`: Generates `CLAUDE.md` for Anthropic Claude.
-   `cursor`: Generates rules for Cursor in `.cursor/rules/`.
-   `copilot`: Generates `.github/copilot-instructions.md` for GitHub Copilot.
-   `gemini`: Generates `GEMINI.md` for Google Gemini.
-   `windsurf`: Generates rules for Windsurf in `.windsurf/`.
-   `continue`: Generates rules and prompts for Continue.dev.
-   `cline`: Generates rules for Cline in `.clinerules/`.
-   `amp` / `codex` / `opencode`: Generates `AGENTS.md` for Sourcegraph tools and OpenCode.

### Using Outputs (Advanced)

If you need more control than presets offer, use the `outputs` key. This is useful for:

-   Generating files for custom or unsupported tools.
-   Using custom file paths or naming schemes.
-   Applying custom templates to the generated files.

See the `output` Object reference below for all available options.

### Combining Presets and Outputs

You can use both `presets` and `outputs` in the same configuration file. This is useful when you want to use a standard preset but need to add or modify one specific output.

When both are used:

1.  The outputs from the `presets` are generated first.
2.  The `outputs` list is then added.
3.  If there is a path conflict (e.g., you use the `claude` preset but also define an `output` with the path `CLAUDE.md`), the entry from the **`outputs` list will take precedence**.

```yaml
# Use the popular preset, but override the Claude file with a custom template
presets:
  - "popular"

outputs:
  - path: "CLAUDE.md" # This will override the default Claude output from the "popular" preset
    template:
      type: "file"
      value: "./my-custom-claude-template.tmpl"
```

### Supporting Custom Tools

What if your favorite tool isn't on the preset list? No problem. `ai-rulez` is designed to be extensible to any tool that consumes text-based configuration files.

The key is the `outputs` list. By defining an `output`, you can tell `ai-rulez` to generate a file at any path with any content you need. This is where the power of templates comes in.

**Example: Supporting a Custom Linter**

Imagine you use a custom linter called "CodeSniffer" that reads its configuration from a `.codesniffer.json` file. You want to provide it with a list of important directories to scan, which you've defined in your `rules`.

Here’s how you would configure that in `ai-rulez.yml`:

```yaml
rules:
  - name: "Important Directories"
    content: "src/"
  - name: "Other Directories"
    content: "pkg/"

outputs:
  - path: ".codesniffer.json"
    template:
      type: "inline"
      value: |
        {
          "scan_directories": [
            {{- range $i, $rule := .Rules -}}
              {{- if $i -}},{{- end -}}
              "{{- .Content -}}"
            {{- end -}}
          ]
        }
```

This configuration would generate a `.codesniffer.json` file with the following content:

```json
{
  "scan_directories": [
    "src/",
    "pkg/"
  ]
}
```

This demonstrates how you can use the `outputs` key and Go's templating engine to generate configuration for any tool, in any format.

---

## Object Reference

This section details the fields for each type of object in the configuration.

### `output` Object

Defines a file or directory to be generated. This is the advanced alternative to using `presets`.

- `path` (string, required): The output path. If it ends with a `/`, it is treated as a directory.
- `type` (string): For directory outputs, specifies the content type to generate (`rule` or `agent`).
- `naming_scheme` (string): For directory outputs, a pattern for filenames (e.g., `{name}.md`).
- `template` (object): A structured object to define the template.
  - `type` (string, required): `builtin`, `file`, or `inline`.
  - `value` (string, required): The name, path, or content of the template.

!!! info "Template Format"
    Templates must always use the structured object format with `type` and `value` fields. String-based templates are no longer supported as of v2.0.

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
  - `type` (string, required): `builtin`, `file`, or `inline`.
  - `value` (string, required): The template content, file path, or builtin name.
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
