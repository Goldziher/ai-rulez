package handlers

import (
	"context"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var ListRulesHandler = CreateListHandler("rules", true)
var GetRuleHandler = CreateGetHandler("rules")
var DeleteRuleHandler = CreateDeleteHandler("rules")

func AddRuleHandler(ctx context.Context, request *ToolRequest) (*mcp.CallToolResult, error) {
	fields, errorResult := ExtractCommonFields(request)
	if errorResult != nil {
		return errorResult, nil
	}

	newRule := &config.Rule{
		ID:       fields.ID,
		Name:     fields.Name,
		Content:  fields.Content,
		Priority: fields.Priority,
		Targets:  fields.Targets,
	}

	return crud.HandleAddMCP(ctx, "rules", newRule)
}

func UpdateRuleHandler(ctx context.Context, request *ToolRequest) (*mcp.CallToolResult, error) {
	name := request.GetString("name", "")

	updates, errorResult := ExtractUpdateFields(request)
	if errorResult != nil {
		return errorResult, nil
	}

	return crud.HandleUpdateMCP(ctx, "rules", name, updates)
}
