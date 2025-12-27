package mcp

import (
	"context"

	"github.com/Goldziher/ai-rulez/internal/mcp/handlers"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type handlerFunc func(context.Context, *handlers.ToolRequest) (*sdkmcp.CallToolResult, error)

type toolSchemaBuilder struct {
	properties map[string]any
	required   []string
}

func newSchemaBuilder() *toolSchemaBuilder {
	return &toolSchemaBuilder{
		properties: map[string]any{},
	}
}

func (b *toolSchemaBuilder) String(name, description string, required bool) *toolSchemaBuilder {
	prop := map[string]any{
		"type": "string",
	}
	if description != "" {
		prop["description"] = description
	}
	b.properties[name] = prop
	if required {
		b.required = append(b.required, name)
	}
	return b
}

func (b *toolSchemaBuilder) StringArray(name, description string, required bool) *toolSchemaBuilder {
	prop := map[string]any{
		"type":  "array",
		"items": map[string]any{"type": "string"},
	}
	if description != "" {
		prop["description"] = description
	}
	b.properties[name] = prop
	if required {
		b.required = append(b.required, name)
	}
	return b
}

func (b *toolSchemaBuilder) Boolean(name, description string, required bool) *toolSchemaBuilder {
	prop := map[string]any{
		"type": "boolean",
	}
	if description != "" {
		prop["description"] = description
	}
	b.properties[name] = prop
	if required {
		b.required = append(b.required, name)
	}
	return b
}

func (b *toolSchemaBuilder) Number(name, description string, required bool) *toolSchemaBuilder {
	prop := map[string]any{
		"type": "number",
	}
	if description != "" {
		prop["description"] = description
	}
	b.properties[name] = prop
	if required {
		b.required = append(b.required, name)
	}
	return b
}

func (b *toolSchemaBuilder) Object(name, description string, required bool) *toolSchemaBuilder {
	prop := map[string]any{
		"type":                 "object",
		"additionalProperties": map[string]any{"type": "string"},
	}
	if description != "" {
		prop["description"] = description
	}
	b.properties[name] = prop
	if required {
		b.required = append(b.required, name)
	}
	return b
}

func (b *toolSchemaBuilder) Build() map[string]any {
	schema := map[string]any{
		"type":       "object",
		"properties": b.properties,
	}
	if len(b.required) > 0 {
		schema["required"] = b.required
	}
	return schema
}

func newTool(name, description string, builder *toolSchemaBuilder) *sdkmcp.Tool {
	var schema map[string]any
	if builder != nil {
		schema = builder.Build()
	} else {
		schema = newSchemaBuilder().Build()
	}
	return &sdkmcp.Tool{
		Name:        name,
		Description: description,
		InputSchema: schema,
	}
}

func (s *Server) addTool(tool *sdkmcp.Tool, handler handlerFunc) {
	if tool.InputSchema == nil {
		tool.InputSchema = newSchemaBuilder().Build()
	}

	sdkmcp.AddTool(s.mcpServer, tool, func(ctx context.Context, req *sdkmcp.CallToolRequest, input map[string]any) (*sdkmcp.CallToolResult, any, error) {
		wrapper := handlers.NewToolRequest(req, input)
		res, err := handler(ctx, wrapper)
		return res, nil, err
	})
}

func (s *Server) registerTools() {
	s.registerProjectTools()
	s.registerUtilityTools()
}


func (s *Server) registerProjectTools() {
	s.addTool(
		newTool("generate_outputs", "Generate output files from the current configuration, respecting includes and extends",
			newSchemaBuilder().
				String("config_file", "Path to the root configuration file (optional)", false),
		),
		handlers.GenerateOutputsHandler,
	)

	s.addTool(
		newTool("validate_config", "Validate the configuration file, including all includes",
			newSchemaBuilder().
				String("config_file", "Path to the root configuration file to validate (optional)", false),
		),
		handlers.ValidateConfigHandler,
	)

	s.addTool(
		newTool("init_project", "Initialize a new ai-rulez project in the current directory",
			newSchemaBuilder().
				String("project_name", "The name for the new project", false).
				StringArray("providers", "A list of providers to enable (e.g., ['claude', 'cursor'])", false).
				Boolean("with_agents", "Include sample agent configurations", false).
				Boolean("all_providers", "Enable all supported providers", false).
				Boolean("popular_providers", "Enable a curated list of popular providers", false),
		),
		handlers.InitProjectHandler,
	)
}

func (s *Server) registerUtilityTools() {
	s.addTool(
		newTool("get_version", "Get the ai-rulez version", nil),
		handlers.GetVersionHandler(s.version),
	)
}
