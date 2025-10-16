package handlers

import (
	"context"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var ListAgentsHandler = CreateListHandler("agents", false)
var GetAgentHandler = CreateGetHandler("agents")
var DeleteAgentHandler = CreateDeleteHandler("agents")

func AddAgentHandler(ctx context.Context, request *ToolRequest) (*mcp.CallToolResult, error) {
	fields, errorResult := ExtractAgentFields(request)
	if errorResult != nil {
		return errorResult, nil
	}

	newAgent := &config.Agent{
		ID:           fields.ID,
		Name:         fields.Name,
		Description:  fields.Description,
		Priority:     fields.Priority,
		Tools:        fields.Tools,
		SystemPrompt: fields.SystemPrompt,
		Template:     crud.CreateTemplateConfig(fields.TemplateType, fields.TemplateValue),
		Targets:      fields.Targets,
	}

	return crud.HandleAddMCP(ctx, "agents", newAgent)
}

func UpdateAgentHandler(ctx context.Context, request *ToolRequest) (*mcp.CallToolResult, error) {
	name := request.GetString("name", "")

	updates, errorResult := ExtractAgentUpdateFields(request)
	if errorResult != nil {
		return errorResult, nil
	}

	return crud.HandleUpdateMCP(ctx, "agents", name, updates)
}
