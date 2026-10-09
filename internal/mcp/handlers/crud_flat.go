package handlers

import (
	"context"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Goldziher/ai-rulez/v5/internal/crud"
)

// The agent and command tools. Both kinds are flat markdown files whose
// frontmatter carries their own settings, so they take no priority or targets.

// flatKind describes one flat content kind.
type flatKind struct {
	// ftype is the content directory: crud.ContentTypeAgents or ContentTypeCommands.
	ftype string
	// singular and plural name the kind in tool names and messages.
	singular, plural string
	add              func(*crud.OperatorImpl, context.Context, *crud.AddFileRequest) (*crud.FileResult, error)
}

var (
	agentKind   = flatKind{ftype: crud.ContentTypeAgents, singular: "agent", plural: "agents", add: (*crud.OperatorImpl).AddAgent}
	commandKind = flatKind{ftype: crud.ContentTypeCommands, singular: "command", plural: "commands", add: (*crud.OperatorImpl).AddCommand}
)

func (k flatKind) create(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := contentOperator(request)
	if err != nil {
		return ToolError(err)
	}
	result, err := k.add(op, ctx, &crud.AddFileRequest{
		Domain:      request.GetString("domain", ""),
		Name:        request.GetString("name", ""),
		Description: request.GetString("description", ""),
		Content:     request.GetString("content", ""),
	})
	if err != nil {
		return ToolError(err)
	}
	return ToolSuccess(map[string]interface{}{
		keySuccess: true, keyOperation: "create_" + k.singular, keyPath: result.FullPath,
		keyName: result.Name, keyDomain: result.Domain, keyMessage: capitalize(k.singular) + " created successfully",
	})
}

func (k flatKind) read(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := contentOperator(request)
	if err != nil {
		return ToolError(err)
	}
	name, domain := request.GetString("name", ""), request.GetString("domain", "")
	content, path, err := readFileContent(ctx, op, domain, k.ftype, name)
	if err != nil {
		return ToolError(err)
	}
	return ToolSuccess(map[string]interface{}{
		keySuccess: true, keyOperation: "read_" + k.singular, keyName: name, keyDomain: domain, keyPath: path, keyContent: content,
	})
}

func (k flatKind) update(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := contentOperator(request)
	if err != nil {
		return ToolError(err)
	}
	result, err := op.UpdateFlatItem(ctx, request.GetString("domain", ""), k.ftype, request.GetString("name", ""), request.GetString("content", ""))
	if err != nil {
		return ToolError(err)
	}
	return ToolSuccess(map[string]interface{}{
		keySuccess: true, keyOperation: "update_" + k.singular, keyPath: result.FullPath,
		keyName: result.Name, keyDomain: result.Domain, keyMessage: capitalize(k.singular) + " updated successfully",
	})
}

func (k flatKind) remove(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := contentOperator(request)
	if err != nil {
		return ToolError(err)
	}
	name, domain := request.GetString("name", ""), request.GetString("domain", "")
	if err := op.RemoveFile(ctx, domain, k.ftype, name); err != nil {
		return ToolError(err)
	}
	return ToolSuccess(map[string]interface{}{
		keySuccess: true, keyOperation: "delete_" + k.singular, keyName: name, keyDomain: domain,
		keyMessage: capitalize(k.singular) + " deleted successfully",
	})
}

func (k flatKind) list(ctx context.Context, request *ToolRequest) (*sdkmcp.CallToolResult, error) {
	op, err := contentOperator(request)
	if err != nil {
		return ToolError(err)
	}
	domain := request.GetString("domain", "")
	files, err := op.ListFiles(ctx, domain, k.ftype)
	if err != nil {
		return ToolError(err)
	}
	return ToolSuccess(map[string]interface{}{
		keySuccess: true, keyOperation: "list_" + k.plural, keyDomain: domain, k.plural: files, keyCount: len(files),
	})
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// Handlers of the agent and command tools.
var (
	CreateAgentHandler = agentKind.create
	ReadAgentHandler   = agentKind.read
	UpdateAgentHandler = agentKind.update
	DeleteAgentHandler = agentKind.remove
	ListAgentsHandler  = agentKind.list

	CreateCommandHandler = commandKind.create
	ReadCommandHandler   = commandKind.read
	UpdateCommandHandler = commandKind.update
	DeleteCommandHandler = commandKind.remove
	ListCommandsHandler  = commandKind.list
)
