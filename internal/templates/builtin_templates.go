package templates

const (
	defaultTemplate = `# {{.ProjectName}}
{{- if .Description}}

{{.Description}}
{{- end}}
{{- if .Version}}

Version: {{.Version}}
{{- end}}

## Governance

- Source of truth: {{if .ConfigFile}}{{.ConfigFile}}{{else}}ai-rulez.yaml{{end}}
- Generated output: {{if .OutputFile}}{{.OutputFile}}{{else}}(preview output){{end}}
- Update workflow:
  1. Edit the source configuration above.
  2. Run ai-rulez generate to refresh generated files.
  3. Commit the regenerated files alongside the configuration change.
- AI assistants must propose edits to the source configuration, not this file.

{{- if .Rules}}

## Rules
{{- range .Rules}}

### {{.Name}}
**Priority:** {{.Priority}}

{{.Content}}
{{- end}}
{{- end}}

{{- if .Sections}}

## Sections
{{- range .Sections}}

### {{.Name}}
**Priority:** {{.Priority}}

{{.Content}}
{{- end}}
{{- end}}

{{- if .Agents}}

## Agents
{{- range .Agents}}

### {{.Name}}
**Priority:** {{.Priority}}

{{.Description}}
{{- if .SystemPrompt}}

#### System Prompt
{{.SystemPrompt}}
{{- end}}
{{- end}}
{{- end}}

{{- if .Commands}}

## Commands
{{- range .Commands}}

### {{.Name}}
**Description:** {{.Description}}
{{- if .Usage}}

- Usage: {{.Usage}}
{{- end}}
{{- if .SystemPrompt}}
- System prompt:
{{.SystemPrompt}}
{{- end}}
{{- end}}
{{- end}}

{{- if .MCPServers}}

## MCP Servers
{{- range .MCPServers}}

### {{.Name}}
{{- if .Description}}
{{.Description}}
{{- end}}
- Transport: {{.GetTransport}}
{{- if .Command}}
- Command: {{.Command}}
{{- end}}
{{- if .Args}}
- Args: {{range $i, $arg := .Args}}{{if $i}}, {{end}}{{$arg}}{{end}}
{{- end}}
{{- if .URL}}
- URL: {{.URL}}
{{- end}}
{{- if .Env}}
- Env:
{{- range $key, $value := .Env}}
  - {{$key}}={{$value}}
{{- end}}
{{- end}}
{{- end}}
{{- end}}
`

	minimalTemplate = `{{range .Rules}}{{.Content}}

{{end}}`

	documentationTemplate = `# {{.ProjectName}} Documentation
{{- if .Description}}

{{.Description}}
{{- end}}

Generated on {{.Timestamp.Format "2006-01-02 15:04:05"}}

{{- range .Sections}}
## {{.Name}}

{{.Content}}

{{- end}}
{{- range .Rules}}
## {{.Name}}

**Priority:** {{.Priority}}

{{.Content}}

{{- end}}
`

	claudeCodeMCPTemplate = `{
  "mcpServers": {
{{- range $i, $server := .MCPServers}}
{{- if ne $i 0}},{{end}}
    "{{$server.Name}}": {
      "type": "{{$server.GetTransport}}"
{{- if and (ne $server.GetTransport "http") (ne $server.GetTransport "sse")}}{{if $server.Command}},
      "command": "{{$server.Command}}"{{end}}
{{- if $server.Args}},
      "args": [{{range $j, $arg := $server.Args}}{{if ne $j 0}}, {{end}}"{{$arg}}"{{end}}]{{end}}
{{- if $server.Env}},
      "env": {
{{- range $key, $value := $server.Env}}
        "{{$key}}": "{{$value}}"{{end}}
      }{{end}}
{{- else}}{{if $server.URL}},
      "url": "{{$server.URL}}"{{end}}
{{- end}}
    }
{{- end}}
  }
}`

	cursorMCPTemplate = `{
  "McpServers": {
{{- range $i, $server := .MCPServers}}
{{- if ne $i 0}},{{end}}
    "{{$server.Name}}": {
{{- if or $server.Command (eq $server.GetTransport "stdio")}}
      "command": "{{$server.Command}}"
{{- if $server.Args}},
      "args": [{{range $j, $arg := $server.Args}}{{if ne $j 0}}, {{end}}"{{$arg}}"{{end}}]{{end}}
{{- if $server.Env}},
      "env": {
{{- range $key, $value := $server.Env}}
        "{{$key}}": "{{$value}}"{{end}}
      }{{end}}
{{- else if $server.URL}}
      "url": "{{$server.URL}}"
{{- end}}
    }
{{- end}}
  }
}`

	windsurfMCPTemplate = `{
  "mcpServers": {
{{- range $i, $server := .MCPServers}}
{{- if ne $i 0}},{{end}}
    "{{$server.Name}}": {
      "command": "{{$server.Command}}"
{{- if $server.Args}},
      "args": [{{range $j, $arg := $server.Args}}{{if ne $j 0}}, {{end}}"{{$arg}}"{{end}}]{{end}}
{{- if $server.Env}},
      "env": {
{{- range $key, $value := $server.Env}}
        "{{$key}}": "{{$value}}"{{end}}
      }{{end}}
    }
{{- end}}
  }
}`

	vscodeMCPTemplate = `{
  "servers": {
{{- range $i, $server := .MCPServers}}
{{- if ne $i 0}},{{end}}
    "{{$server.Name}}": {
      "type": "{{$server.GetTransport}}"
{{- if or $server.Command (eq $server.GetTransport "stdio")}},
      "command": "{{$server.Command}}"
{{- if $server.Args}},
      "args": [{{range $j, $arg := $server.Args}}{{if ne $j 0}}, {{end}}"{{$arg}}"{{end}}]{{end}}
{{- else if $server.URL}},
      "url": "{{$server.URL}}"
{{- end}}
    }
{{- end}}
  }
}`

	continuedevMCPTemplate = `name: {{.ProjectName}} MCP Configuration
version: 1.0.0
schema: v1
mcpServers:
{{- range .MCPServers}}
- name: {{.Name}}
{{- if .Description}}
  description: {{.Description}}
{{- end}}
{{- if ne .GetTransport "stdio"}}
  type: {{if eq .GetTransport "http"}}streamable-http{{else}}{{.GetTransport}}{{end}}
{{- end}}
{{- if or .Command (eq .GetTransport "stdio")}}
  command: {{.Command}}
{{- if .Args}}
  args:
{{- range .Args}}
  - "{{.}}"
{{- end}}
{{- end}}
{{- end}}
{{- if .URL}}
  url: {{.URL}}
{{- end}}
{{- if .Env}}
  env:
{{- range $key, $value := .Env}}
    {{$key}}: {{$value}}
{{- end}}
{{- end}}
{{- end}}
`

	clineMCPTemplate = `{
  "mcpServers": {
{{- range $i, $server := .MCPServers}}
{{- if ne $i 0}},{{end}}
    "{{$server.Name}}": {
{{- if $server.Command}}
      "command": "{{$server.Command}}"
{{- if $server.Args}},
      "args": [{{range $j, $arg := $server.Args}}{{if ne $j 0}}, {{end}}"{{$arg}}"{{end}}]{{end}}
{{- if $server.Env}},
      "env": {
{{- range $key, $value := $server.Env}}
        "{{$key}}": "{{$value}}"{{end}}
      }{{end}},
{{- end}}
      "disabled": {{if $server.IsEnabled}}false{{else}}true{{end}}
    }
{{- end}}
  }
}`

	geminiMCPTemplate = `{
  "mcpServers": {
{{- range $i, $server := .MCPServers}}
{{- if ne $i 0}},{{end}}
    "{{$server.Name}}": {
      "type": "{{$server.GetTransport}}"
{{- if and (ne $server.GetTransport "http") (ne $server.GetTransport "sse")}}{{if $server.Command}},
      "command": "{{$server.Command}}"{{end}}
{{- if $server.Args}},
      "args": [{{range $j, $arg := $server.Args}}{{if ne $j 0}}, {{end}}"{{$arg}}"{{end}}]{{end}}
{{- if $server.Env}},
      "env": {
{{- range $key, $value := $server.Env}}
        "{{$key}}": "{{$value}}"{{end}}
      }{{end}}
{{- else}}{{if $server.URL}},
      "url": "{{$server.URL}}"{{end}}
{{- end}}
    }
{{- end}}
  }
}`

	continuedevPromptsTemplate = `# 🤖 GENERATED FILE - DO NOT EDIT DIRECTLY
# This file was generated by ai-rulez.
# To update, edit your ai_rulez.yaml and run 'ai-rulez generate'.
# Place this file in .continue/prompts/ directory

name: AI Rulez Custom Prompts
version: 1.0.0
schema: v1

prompts:
{{- range .Agents}}
  - name: {{.Name}}
    description: {{.Description}}
    prompt: |
{{- if .SystemPrompt}}
      {{.SystemPrompt}}
{{- else if .Instructions}}
      {{.Instructions}}
{{- else}}
      You are {{.Name}}. {{.Description}}
{{- end}}
{{- end}}
`

	claudeLightweightTemplate = `<!--
🤖 AI-RULEZ :: GENERATED FILE — DO NOT EDIT DIRECTLY
Project: {{.ProjectName}}
Generated: {{.Timestamp.Format "2006-01-02 15:04:05"}}
Source of truth: {{if .ConfigFile}}{{.ConfigFile}}{{else}}ai-rulez.yaml{{end}}
Target file: {{if .OutputFile}}{{.OutputFile}}{{else}}CLAUDE.md{{end}}
{{- if or .Rules .Sections .Agents}}
Content summary: rules={{len .Rules}}, sections={{len .Sections}}, agents={{len .Agents}}
{{- end}}

UPDATE WORKFLOW
1. Modify {{if .ConfigFile}}{{.ConfigFile}}{{else}}ai-rulez.yaml{{end}}
2. Run ` + "`ai-rulez generate`" + ` to refresh generated files
3. Commit regenerated outputs together with the config changes

AI ASSISTANT SAFEGUARDS
- Treat {{if .ConfigFile}}{{.ConfigFile}}{{else}}ai-rulez.yaml{{end}} as the canonical configuration
- Never overwrite {{if .OutputFile}}{{.OutputFile}}{{else}}CLAUDE.md{{end}} manually; regenerate instead
- Surface changes as patches to {{if .ConfigFile}}{{.ConfigFile}}{{else}}ai-rulez.yaml{{end}} (include doc/test updates)

Need help? /capability-plan or https://github.com/Goldziher/ai-rulez
-->

# {{.ProjectName}}
{{- if .Description}}

{{.Description}}
{{- end}}
{{- if .Version}}

Version: {{.Version}}
{{- end}}

## Governance

- Source of truth: {{if .ConfigFile}}{{.ConfigFile}}{{else}}ai-rulez.yaml{{end}}
- Generated output: {{if .OutputFile}}{{.OutputFile}}{{else}}CLAUDE.md{{end}}
- Update workflow:
  1. Edit the source configuration above.
  2. Run ai-rulez generate to refresh generated files.
  3. Commit the regenerated files alongside the configuration change.
- AI assistants must propose edits to the source configuration, not this file.
{{- if .Rules}}

## Rules
{{- range .Rules}}

### {{.Name}}
**Priority:** {{.Priority}}

{{.Content}}
{{- end}}
{{- end}}
{{- if .Sections}}

## Sections
{{- range .Sections}}

### {{.Name}}
**Priority:** {{.Priority}}

{{.Content}}
{{- end}}
{{- end}}
{{- if .Agents}}

## Agents
{{- range .Agents}}

### {{.Name}}
**Priority:** {{.Priority}}

{{.Description}}
{{- if .SystemPrompt}}

{{.SystemPrompt}}
{{- end}}
{{- end}}
{{- end}}
{{- if .MCPServers}}

## MCP Servers
{{- range .MCPServers}}

### {{.Name}}
{{- if .Description}}
{{.Description}}
{{- end}}
- Transport: {{.GetTransport}}
{{- if .Command}}
- Command: {{.Command}}
{{- end}}
{{- if .Args}}
- Args: {{range $i, $arg := .Args}}{{if $i}}, {{end}}{{$arg}}{{end}}
{{- end}}
{{- if .URL}}
- URL: {{.URL}}
{{- end}}
{{- if .Env}}
- Env:
{{- range $key, $value := .Env}}
  - {{$key}}={{$value}}
{{- end}}
{{- end}}
{{- end}}
{{- end}}
`
)

func getBuiltinTemplates() map[string]string {
	return map[string]string{
		"default":             defaultTemplate,
		"minimal":             minimalTemplate,
		"documentation":       documentationTemplate,
		"claude-code-mcp":     claudeCodeMCPTemplate,
		"claude-lightweight":  claudeLightweightTemplate,
		"cursor-mcp":          cursorMCPTemplate,
		"windsurf-mcp":        windsurfMCPTemplate,
		"vscode-mcp":          vscodeMCPTemplate,
		"continuedev-mcp":     continuedevMCPTemplate,
		"cline-mcp":           clineMCPTemplate,
		"gemini-mcp":          geminiMCPTemplate,
		"continuedev-prompts": continuedevPromptsTemplate,
	}
}
