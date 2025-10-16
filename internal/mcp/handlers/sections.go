package handlers

import (
	"context"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var ListSectionsHandler = CreateListHandler("sections", false)
var GetSectionHandler = CreateGetHandler("sections")
var DeleteSectionHandler = CreateDeleteHandler("sections")

func AddSectionHandler(ctx context.Context, request *ToolRequest) (*mcp.CallToolResult, error) {
	fields, errorResult := ExtractCommonFields(request)
	if errorResult != nil {
		return errorResult, nil
	}

	newSection := &config.Section{
		ID:       fields.ID,
		Name:     fields.Name,
		Content:  fields.Content,
		Priority: fields.Priority,
		Targets:  fields.Targets,
	}

	return crud.HandleAddMCP(ctx, "sections", newSection)
}

func UpdateSectionHandler(ctx context.Context, request *ToolRequest) (*mcp.CallToolResult, error) {
	name := request.GetString("name", "")

	updates, errorResult := ExtractUpdateFields(request)
	if errorResult != nil {
		return errorResult, nil
	}

	return crud.HandleUpdateMCP(ctx, "sections", name, updates)
}
