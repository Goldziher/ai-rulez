package handlers

import (
	"context"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/mark3labs/mcp-go/mcp"
)

func CreateListHandler(entityType string, supportsFilter bool) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if supportsFilter {
			nameFilter := request.GetString("name_filter", "")
			return crud.HandleListMCPWithFilter(ctx, entityType, nameFilter)
		}
		return crud.HandleListMCP(ctx, entityType)
	}
}

func CreateGetHandler(entityType string) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := request.GetString("name", "")
		return crud.HandleGetMCP(ctx, entityType, name)
	}
}

func CreateGetOutputHandler() func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path := request.GetString("path", "")
		return crud.HandleGetMCP(ctx, "outputs", path)
	}
}

func CreateDeleteHandler(entityType string) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := request.GetString("name", "")
		return crud.HandleDeleteMCP(ctx, entityType, name)
	}
}

type CommonFields struct {
	ID       string
	Name     string
	Content  string
	Priority config.Priority
	Targets  []string
}

func ExtractCommonFields(request mcp.CallToolRequest) (*CommonFields, *mcp.CallToolResult) {
	fields := &CommonFields{
		ID:      request.GetString("id", ""),
		Name:    request.GetString("name", ""),
		Content: request.GetString("content", ""),
		Targets: request.GetStringSlice("targets", []string{}),
	}

	priorityStr := request.GetString("priority", "")
	if priorityStr != "" {
		priority, err := config.ParsePriority(priorityStr)
		if err != nil {
			errorResult, _ := crud.ToolError(err) //nolint:errcheck
			return nil, errorResult
		}
		fields.Priority = priority
	}

	return fields, nil
}

func ExtractUpdateFields(request mcp.CallToolRequest) (map[string]interface{}, *mcp.CallToolResult) {
	updates := make(map[string]interface{})

	if newName := request.GetString("new_name", ""); newName != "" {
		updates["Name"] = newName
	}
	if id := request.GetString("id", ""); id != "" {
		updates["ID"] = id
	}
	if content := request.GetString("content", ""); content != "" {
		updates["Content"] = content
	}
	if priorityStr := request.GetString("priority", ""); priorityStr != "" {
		priority, err := config.ParsePriority(priorityStr)
		if err != nil {
			errorResult, _ := crud.ToolError(err) //nolint:errcheck
			return nil, errorResult
		}
		updates["Priority"] = priority
	}
	if targets := request.GetStringSlice("targets", nil); targets != nil {
		updates["Targets"] = targets
	}

	return updates, nil
}

type AgentFields struct {
	CommonFields
	Description   string
	Tools         []string
	Model         string
	SystemPrompt  string
	TemplateType  string
	TemplateValue string
}

func ExtractAgentFields(request mcp.CallToolRequest) (*AgentFields, *mcp.CallToolResult) {
	common, errorResult := ExtractCommonFields(request)
	if errorResult != nil {
		return nil, errorResult
	}

	fields := &AgentFields{
		CommonFields:  *common,
		Description:   request.GetString("description", ""),
		Tools:         request.GetStringSlice("tools", []string{}),
		Model:         request.GetString("model", ""),
		SystemPrompt:  request.GetString("system_prompt", ""),
		TemplateType:  request.GetString("template_type", ""),
		TemplateValue: request.GetString("template_value", ""),
	}

	return fields, nil
}

func ExtractAgentUpdateFields(request mcp.CallToolRequest) (map[string]interface{}, *mcp.CallToolResult) {
	updates, errorResult := ExtractUpdateFields(request)
	if errorResult != nil {
		return nil, errorResult
	}

	if description := request.GetString("description", ""); description != "" {
		updates["Description"] = description
	}
	if tools := request.GetStringSlice("tools", nil); tools != nil {
		updates["Tools"] = tools
	}
	if model := request.GetString("model", ""); model != "" {
		updates["Model"] = model
	}
	if systemPrompt := request.GetString("system_prompt", ""); systemPrompt != "" {
		updates["SystemPrompt"] = systemPrompt
	}
	if templateType := request.GetString("template_type", ""); templateType != "" {
		templateValue := request.GetString("template_value", "")
		updates["Template"] = crud.CreateTemplateConfig(templateType, templateValue)
	}

	return updates, nil
}
