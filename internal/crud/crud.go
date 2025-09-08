package crud

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/utils"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/samber/oops"
	"gopkg.in/yaml.v3"
)

const (
	ElementTypeRules      = "rules"
	ElementTypeSections   = "sections"
	ElementTypeAgents     = "agents"
	ElementTypeOutputs    = "outputs"
	ElementTypeCommands   = "commands"
	ElementTypeMCPServers = "mcp_servers"
	ElementTypeIncludes   = "includes"
	ElementTypeExtends    = "extends"
	ElementTypeMetadata   = "metadata"
)

func getSingular(elementType string) string {
	switch elementType {
	case ElementTypeRules:
		return "rule"
	case ElementTypeSections:
		return "section"
	case ElementTypeAgents:
		return "agent"
	case ElementTypeOutputs:
		return "output"
	case ElementTypeCommands:
		return "command"
	case ElementTypeMCPServers:
		return "MCP server"
	case ElementTypeIncludes:
		return "include"
	case ElementTypeExtends:
		return "extends"
	case ElementTypeMetadata:
		return "metadata"
	default:
		return strings.TrimSuffix(elementType, "s")
	}
}

func formatConfigError(err error) string {
	if oopsErr, ok := oops.AsOops(err); ok {
		var details strings.Builder
		details.WriteString(err.Error())

		errContext := oopsErr.Context()
		if errors, exists := errContext["errors"]; exists {
			details.WriteString("\n\nValidation errors:")
			if errorList, ok := errors.([]string); ok {
				for _, validationErr := range errorList {
					details.WriteString("\n")
					details.WriteString(validationErr)
				}
			}
		}

		if hint := oopsErr.Hint(); hint != "" {
			details.WriteString("\n\nHint: ")
			details.WriteString(hint)
		}

		return details.String()
	}
	return err.Error()
}

func AddElement(elementType string, element interface{}) {
	ctx := context.Background()
	cfg, configPath, err := loadConfigWithPath(ctx, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %s\n", formatConfigError(err))
		os.Exit(1)
	}

	_, err = HandleAdd(ctx, elementType, element, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error adding %s: %v\n", elementType, err)
		os.Exit(1)
	}

	if err := saveConfig(cfg, configPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %s\n", formatConfigError(err))
		os.Exit(1)
	}

	fmt.Printf("Added %s\n", getSingular(elementType))
}

func UpdateElement(elementType, name string, updates map[string]interface{}) {
	ctx := context.Background()
	cfg, configPath, err := loadConfigWithPath(ctx, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %s\n", formatConfigError(err))
		os.Exit(1)
	}

	_, err = HandleUpdate(ctx, elementType, name, updates, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error updating %s: %v\n", elementType, err)
		os.Exit(1)
	}

	if err := saveConfig(cfg, configPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %s\n", formatConfigError(err))
		os.Exit(1)
	}

	fmt.Printf("Updated %s\n", getSingular(elementType))
}

func DeleteElement(elementType, name string) {
	ctx := context.Background()
	cfg, configPath, err := loadConfigWithPath(ctx, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %s\n", formatConfigError(err))
		os.Exit(1)
	}

	_, err = HandleDelete(ctx, elementType, name, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error deleting %s: %v\n", elementType, err)
		os.Exit(1)
	}

	if err := saveConfig(cfg, configPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %s\n", formatConfigError(err))
		os.Exit(1)
	}

	fmt.Printf("Deleted %s\n", getSingular(elementType))
}

func GetElement(elementType, name string) {
	ctx := context.Background()
	cfg, _, err := loadConfigWithPath(ctx, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %s\n", formatConfigError(err))
		os.Exit(1)
	}

	result, err := HandleGet(ctx, elementType, name, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting %s: %v\n", elementType, err)
		os.Exit(1)
	}

	if len(result.Content) > 0 {
		if textContent, ok := result.Content[0].(mcp.TextContent); ok {
			fmt.Println(textContent.Text)
		}
	}
}

func ListElements(elementType string) {
	ctx := context.Background()
	var cfg *config.Config
	var err error

	if elementType == ElementTypeIncludes {
		configPath, findErr := config.FindConfigFile(".")
		if findErr != nil {
			fmt.Fprintf(os.Stderr, "Error finding config: %s\n", formatConfigError(findErr))
			os.Exit(1)
		}
		cfg, err = config.LoadConfig(configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config: %s\n", formatConfigError(err))
			os.Exit(1)
		}
	} else {
		cfg, _, err = loadConfigWithPath(ctx, "")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config: %s\n", formatConfigError(err))
			os.Exit(1)
		}
	}

	result, err := HandleList(ctx, elementType, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing %s: %v\n", elementType, err)
		os.Exit(1)
	}

	if len(result.Content) > 0 {
		if textContent, ok := result.Content[0].(mcp.TextContent); ok {
			fmt.Println(textContent.Text)
		}
	}
}

func AddToList(listType, value string) {
	ctx := context.Background()
	var cfg *config.Config
	var configPath string
	var err error

	if listType == "includes" {
		configPath, err = config.FindConfigFile(".")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error finding config: %s\n", formatConfigError(err))
			os.Exit(1)
		}
		cfg, err = config.LoadConfig(configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config: %s\n", formatConfigError(err))
			os.Exit(1)
		}
	} else {
		cfg, configPath, err = loadConfigWithPath(ctx, "")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config: %s\n", formatConfigError(err))
			os.Exit(1)
		}
	}

	_, err = HandleAddToList(ctx, listType, value, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error adding to %s: %v\n", listType, err)
		os.Exit(1)
	}

	if err := saveConfig(cfg, configPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %s\n", formatConfigError(err))
		os.Exit(1)
	}

	fmt.Printf("Added %s\n", getSingular(listType))
}

func DeleteFromList(listType, value string) {
	ctx := context.Background()
	var cfg *config.Config
	var configPath string
	var err error

	if listType == "includes" {
		configPath, err = config.FindConfigFile(".")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error finding config: %s\n", formatConfigError(err))
			os.Exit(1)
		}
		cfg, err = config.LoadConfig(configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config: %s\n", formatConfigError(err))
			os.Exit(1)
		}
	} else {
		cfg, configPath, err = loadConfigWithPath(ctx, "")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config: %s\n", formatConfigError(err))
			os.Exit(1)
		}
	}

	_, err = HandleDeleteFromList(ctx, listType, value, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error deleting from %s: %v\n", listType, err)
		os.Exit(1)
	}

	if err := saveConfig(cfg, configPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %s\n", formatConfigError(err))
		os.Exit(1)
	}

	fmt.Printf("Deleted %s\n", getSingular(listType))
}

func HandleAdd(ctx context.Context, elementType string, element interface{}, cfg *config.Config) (*mcp.CallToolResult, error) {
	switch elementType {
	case ElementTypeRules:
		rule := element.(*config.Rule)
		return genericAdd(RuleOps, *rule, cfg)
	case ElementTypeSections:
		section := element.(*config.Section)
		return genericAdd(SectionOps, *section, cfg)
	case ElementTypeAgents:
		agent := element.(*config.Agent)
		return genericAdd(AgentOps, *agent, cfg)
	case ElementTypeOutputs:
		output := element.(*config.Output)
		return genericAdd(OutputOps, *output, cfg)
	case ElementTypeCommands:
		cmd := element.(*config.Command)
		return genericAdd(CommandOps, *cmd, cfg)
	case ElementTypeMCPServers:
		server := element.(*config.MCPServer)
		return genericAdd(MCPServerOps, *server, cfg)
	default:
		return nil, fmt.Errorf("unsupported element type: %s", elementType)
	}
}

func HandleGet(ctx context.Context, elementType, name string, cfg *config.Config) (*mcp.CallToolResult, error) {
	switch elementType {
	case ElementTypeMetadata:
		return handleGetMetadata(cfg)
	case ElementTypeExtends:
		return handleGetExtends()
	case ElementTypeRules:
		return genericGet(RuleOps, name, cfg)
	case ElementTypeSections:
		return genericGet(SectionOps, name, cfg)
	case ElementTypeAgents:
		return genericGet(AgentOps, name, cfg)
	case ElementTypeOutputs:
		return genericGet(OutputOps, name, cfg)
	case ElementTypeCommands:
		return genericGet(CommandOps, name, cfg)
	case ElementTypeMCPServers:
		return genericGet(MCPServerOps, name, cfg)
	default:
		return nil, fmt.Errorf("unsupported element type: %s", elementType)
	}
}

func handleGetMetadata(cfg *config.Config) (*mcp.CallToolResult, error) {
	var output strings.Builder
	output.WriteString(fmt.Sprintf("Name:        %s\n", cfg.Metadata.Name))
	if cfg.Metadata.Version != "" {
		output.WriteString(fmt.Sprintf("Version:     %s\n", cfg.Metadata.Version))
	}
	if cfg.Metadata.Description != "" {
		output.WriteString(fmt.Sprintf("Description: %s", cfg.Metadata.Description))
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: strings.TrimSpace(output.String()),
			},
		},
	}, nil
}

func handleGetExtends() (*mcp.CallToolResult, error) {
	configPath, err := config.FindConfigFile(".")
	if err != nil {
		configPath = "ai-rulez.yaml"
	}
	rawCfg, err := config.LoadConfig(configPath)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: "Extends: not set",
				},
			},
		}, err
	}
	if rawCfg.Extends == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: "Extends: not set",
				},
			},
		}, nil
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: fmt.Sprintf("Extends: %s", rawCfg.Extends),
			},
		},
	}, nil
}

func HandleList(ctx context.Context, elementType string, cfg *config.Config) (*mcp.CallToolResult, error) {
	switch elementType {
	case ElementTypeRules:
		return genericList(RuleOps, cfg)
	case ElementTypeSections:
		return genericList(SectionOps, cfg)
	case ElementTypeAgents:
		return genericList(AgentOps, cfg)
	case ElementTypeOutputs:
		return genericList(OutputOps, cfg)
	case ElementTypeCommands:
		return genericList(CommandOps, cfg)
	case ElementTypeMCPServers:
		return genericList(MCPServerOps, cfg)
	case ElementTypeIncludes:
		return handleListIncludes(cfg)
	default:
		return nil, fmt.Errorf("unsupported element type: %s", elementType)
	}
}

func handleListIncludes(cfg *config.Config) (*mcp.CallToolResult, error) {
	var items []string
	if len(cfg.Includes) == 0 {
		items = append(items, "No includes found")
	} else {
		for _, include := range cfg.Includes {
			items = append(items, fmt.Sprintf("  %s", include))
		}
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: fmt.Sprintf("Includes (%d):\n%s", len(cfg.Includes), strings.Join(items, "\n")),
			},
		},
	}, nil
}

func HandleUpdate(ctx context.Context, elementType, name string, updates map[string]interface{}, cfg *config.Config) (*mcp.CallToolResult, error) {
	switch elementType {
	case ElementTypeMetadata:
		return handleUpdateMetadata(updates, cfg)
	case ElementTypeExtends:
		return handleUpdateExtends(updates, cfg)
	case ElementTypeRules:
		return genericUpdate(RuleOps, name, updates, cfg)
	case ElementTypeSections:
		return genericUpdate(SectionOps, name, updates, cfg)
	case ElementTypeAgents:
		return genericUpdate(AgentOps, name, updates, cfg)
	case ElementTypeOutputs:
		return genericUpdate(OutputOps, name, updates, cfg)
	case ElementTypeCommands:
		return genericUpdate(CommandOps, name, updates, cfg)
	case ElementTypeMCPServers:
		return genericUpdate(MCPServerOps, name, updates, cfg)
	default:
		return nil, fmt.Errorf("unsupported element type: %s", elementType)
	}
}

func handleUpdateMetadata(updates map[string]interface{}, cfg *config.Config) (*mcp.CallToolResult, error) {
	for key, value := range updates {
		utils.LogIfErr(setFieldValue(&cfg.Metadata, key, value))
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: "Updated metadata",
			},
		},
	}, nil
}

func handleUpdateExtends(updates map[string]interface{}, cfg *config.Config) (*mcp.CallToolResult, error) {
	if extends, ok := updates["Extends"].(string); ok {
		cfg.Extends = extends
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: fmt.Sprintf("Updated extends to: %s", cfg.Extends),
			},
		},
	}, nil
}

func HandleDelete(ctx context.Context, elementType, name string, cfg *config.Config) (*mcp.CallToolResult, error) {
	switch elementType {
	case ElementTypeExtends:
		return handleDeleteExtends(cfg)
	case ElementTypeRules:
		return genericDelete(RuleOps, name, cfg)
	case ElementTypeSections:
		return genericDelete(SectionOps, name, cfg)
	case ElementTypeAgents:
		return genericDelete(AgentOps, name, cfg)
	case ElementTypeOutputs:
		return genericDelete(OutputOps, name, cfg)
	case ElementTypeCommands:
		return genericDelete(CommandOps, name, cfg)
	case ElementTypeMCPServers:
		return genericDelete(MCPServerOps, name, cfg)
	default:
		return nil, fmt.Errorf("unsupported element type: %s", elementType)
	}
}

func handleDeleteExtends(cfg *config.Config) (*mcp.CallToolResult, error) {
	cfg.Extends = ""
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: "Deleted extends configuration",
			},
		},
	}, nil
}

func HandleAddToList(ctx context.Context, listType, value string, cfg *config.Config) (*mcp.CallToolResult, error) {
	switch listType {
	case ElementTypeIncludes:
		cfg.Includes = append(cfg.Includes, value)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Added to includes: %s", value),
				},
			},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported list type: %s", listType)
	}
}

func HandleDeleteFromList(ctx context.Context, listType, value string, cfg *config.Config) (*mcp.CallToolResult, error) {
	switch listType {
	case ElementTypeIncludes:
		for i, include := range cfg.Includes {
			if include == value {
				cfg.Includes = append(cfg.Includes[:i], cfg.Includes[i+1:]...)
				return &mcp.CallToolResult{
					Content: []mcp.Content{
						mcp.TextContent{
							Type: "text",
							Text: fmt.Sprintf("Deleted from includes: %s", value),
						},
					},
				}, nil
			}
		}
		return nil, fmt.Errorf("include not found: %s", value)
	default:
		return nil, fmt.Errorf("unsupported list type: %s", listType)
	}
}

func HandleListMCP(ctx context.Context, elementType string) (*mcp.CallToolResult, error) {
	cfg, err := loadConfig(ctx, "")
	if err != nil {
		return nil, err
	}
	return HandleListJSON(ctx, elementType, cfg)
}

func HandleListMCPWithFilter(ctx context.Context, elementType, nameFilter string) (*mcp.CallToolResult, error) {
	cfg, err := loadConfig(ctx, "")
	if err != nil {
		return nil, err
	}
	return HandleListJSONWithFilter(ctx, elementType, cfg, nameFilter)
}

func HandleGetMCP(ctx context.Context, elementType, name string) (*mcp.CallToolResult, error) {
	cfg, err := loadConfig(ctx, "")
	if err != nil {
		return nil, err
	}
	return HandleGetJSON(ctx, elementType, name, cfg)
}

func HandleListJSON(ctx context.Context, elementType string, cfg *config.Config) (*mcp.CallToolResult, error) {
	switch elementType {
	case ElementTypeRules:
		return genericListJSON(RuleOps, cfg)
	case ElementTypeSections:
		return genericListJSON(SectionOps, cfg)
	case ElementTypeAgents:
		return genericListJSON(AgentOps, cfg)
	case ElementTypeOutputs:
		return genericListJSON(OutputOps, cfg)
	case ElementTypeMCPServers:
		return genericListJSON(MCPServerOps, cfg)
	case ElementTypeCommands:
		return genericListJSON(CommandOps, cfg)
	default:
		return nil, fmt.Errorf("unknown element type: %s", elementType)
	}
}

func HandleListJSONWithFilter(ctx context.Context, elementType string, cfg *config.Config, nameFilter string) (*mcp.CallToolResult, error) {
	switch elementType {
	case ElementTypeRules:
		return genericListJSONWithFilter(RuleOps, cfg, nameFilter)
	case ElementTypeSections:
		return genericListJSONWithFilter(SectionOps, cfg, nameFilter)
	case ElementTypeAgents:
		return genericListJSONWithFilter(AgentOps, cfg, nameFilter)
	case ElementTypeOutputs:
		return genericListJSONWithFilter(OutputOps, cfg, nameFilter)
	default:
		return nil, fmt.Errorf("unknown element type: %s", elementType)
	}
}

func HandleGetJSON(ctx context.Context, elementType, name string, cfg *config.Config) (*mcp.CallToolResult, error) {
	switch elementType {
	case ElementTypeRules:
		return genericGetJSON(RuleOps, name, cfg)
	case ElementTypeSections:
		return genericGetJSON(SectionOps, name, cfg)
	case ElementTypeAgents:
		return genericGetJSON(AgentOps, name, cfg)
	case ElementTypeOutputs:
		return genericGetJSON(OutputOps, name, cfg)
	case ElementTypeMCPServers:
		return genericGetJSON(MCPServerOps, name, cfg)
	case ElementTypeCommands:
		return genericGetJSON(CommandOps, name, cfg)
	default:
		return nil, fmt.Errorf("unknown element type: %s", elementType)
	}
}

func HandleAddMCP(ctx context.Context, elementType string, element interface{}) (*mcp.CallToolResult, error) {
	cfg, configPath, err := loadConfigWithPath(ctx, "")
	if err != nil {
		return nil, err
	}

	var elementName string
	switch elementType {
	case ElementTypeRules:
		rule, _ := element.(*config.Rule) //nolint:errcheck // Type assertion guaranteed by switch
		cfg.Rules = append(cfg.Rules, *rule)
		elementName = rule.Name
	case ElementTypeSections:
		section, _ := element.(*config.Section) //nolint:errcheck // Type assertion guaranteed by switch
		cfg.Sections = append(cfg.Sections, *section)
		elementName = section.Name
	case ElementTypeAgents:
		agent, _ := element.(*config.Agent) //nolint:errcheck // Type assertion guaranteed by switch
		cfg.Agents = append(cfg.Agents, *agent)
		elementName = agent.Name
	case ElementTypeOutputs:
		output, _ := element.(*config.Output) //nolint:errcheck // Type assertion guaranteed by switch
		cfg.Outputs = append(cfg.Outputs, *output)
		elementName = output.Path
	case ElementTypeMCPServers:
		server, _ := element.(*config.MCPServer) //nolint:errcheck // Type assertion guaranteed by switch
		cfg.MCPServers = append(cfg.MCPServers, *server)
		elementName = server.Name
	case ElementTypeCommands:
		command, _ := element.(*config.Command) //nolint:errcheck // Type assertion guaranteed by switch
		cfg.Commands = append(cfg.Commands, *command)
		elementName = command.Name
	default:
		return nil, fmt.Errorf("unknown element type: %s", elementType)
	}

	if err := saveConfig(cfg, configPath); err != nil {
		return nil, err
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: fmt.Sprintf(`{"message": "Added %s: %s"}`, elementType, elementName),
			},
		},
	}, nil
}

func HandleUpdateMCP(ctx context.Context, elementType, name string, updates map[string]interface{}) (*mcp.CallToolResult, error) {
	cfg, configPath, err := loadConfigWithPath(ctx, "")
	if err != nil {
		return nil, err
	}

	_, err = HandleUpdate(ctx, elementType, name, updates, cfg)
	if err != nil {
		return nil, err
	}

	if err := saveConfig(cfg, configPath); err != nil {
		return nil, err
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: fmt.Sprintf(`{"message": "Updated %s: %s"}`, elementType, name),
			},
		},
	}, nil
}

func HandleDeleteMCP(ctx context.Context, elementType, name string) (*mcp.CallToolResult, error) {
	cfg, configPath, err := loadConfigWithPath(ctx, "")
	if err != nil {
		return nil, err
	}

	_, err = HandleDelete(ctx, elementType, name, cfg)
	if err != nil {
		return nil, err
	}

	if err := saveConfig(cfg, configPath); err != nil {
		return nil, err
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: fmt.Sprintf(`{"message": "Deleted %s: %s"}`, elementType, name),
			},
		},
	}, nil
}

func HandleAddToListMCP(ctx context.Context, listType, value string) (*mcp.CallToolResult, error) {
	cfg, configPath, err := loadConfigWithPath(ctx, "")
	if err != nil {
		return nil, err
	}

	result, err := HandleAddToList(ctx, listType, value, cfg)
	if err != nil {
		return nil, err
	}

	if err := saveConfig(cfg, configPath); err != nil {
		return nil, err
	}

	return result, nil
}

func HandleDeleteFromListMCP(ctx context.Context, listType, value string) (*mcp.CallToolResult, error) {
	cfg, configPath, err := loadConfigWithPath(ctx, "")
	if err != nil {
		return nil, err
	}

	result, err := HandleDeleteFromList(ctx, listType, value, cfg)
	if err != nil {
		return nil, err
	}

	if err := saveConfig(cfg, configPath); err != nil {
		return nil, err
	}

	return result, nil
}

func ParseEnvVars(envVars []string) map[string]string {
	env := make(map[string]string)
	for _, envVar := range envVars {
		parts := strings.SplitN(envVar, "=", 2)
		if len(parts) == 2 {
			env[parts[0]] = parts[1]
		}
	}
	return env
}

func FmtError(format string, args ...interface{}) {
	fmt.Printf("Error: "+format+"\n", args...)
}

func CreateTemplateConfig(templateType, templateValue string) config.TemplateConfig {
	if templateType == "" || templateValue == "" {
		return nil
	}

	switch templateType {
	case "builtin":
		return map[string]interface{}{
			"type":  "builtin",
			"value": templateValue,
		}
	case "file":
		return map[string]interface{}{
			"type":  "file",
			"value": templateValue,
		}
	case "inline":
		return map[string]interface{}{
			"type":  "inline",
			"value": templateValue,
		}
	default:
		return map[string]interface{}{
			"type":  "builtin",
			"value": templateValue,
		}
	}
}

func ToolError(err error) (*mcp.CallToolResult, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: fmt.Sprintf("Error: %v", err),
			},
		},
		IsError: true,
	}, nil
}

func ToolSuccess(result interface{}) (*mcp.CallToolResult, error) {
	text := fmt.Sprintf("%v", result)
	if data, err := json.Marshal(result); err == nil {
		text = string(data)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: text,
			},
		},
	}, nil
}

func CreateContinueDevConfig() error {
	continueDir := ".continue"
	if err := os.MkdirAll(continueDir, 0o755); err != nil {
		return fmt.Errorf("failed to create .continue directory: %w", err)
	}

	configContent := `{
  "models": [
    {
      "title": "Claude 3.5 Sonnet",
      "provider": "anthropic",
      "model": "claude-3-5-sonnet-20241022",
      "apiKey": "[API_KEY]"
    }
  ],
  "customCommands": [
    {
      "name": "test",
      "prompt": "{{{ input }}}\n\nWrite comprehensive unit tests for the selected code. Ensure the tests cover all edge cases and functions.",
      "description": "Write unit tests for the selected code"
    }
  ],
  "contextProviders": [
    {
      "name": "code",
      "params": {}
    },
    {
      "name": "docs",
      "params": {}
    }
  ],
  "slashCommands": [
    {
      "name": "edit",
      "description": "Edit the selected code"
    },
    {
      "name": "comment",
      "description": "Add comments to the selected code"
    }
  ]
}`

	configPath := filepath.Join(continueDir, "config.json")
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		return fmt.Errorf("failed to write continue config: %w", err)
	}

	return nil
}

func loadConfig(ctx context.Context, configFile string) (*config.Config, error) {
	cfg, _, err := loadConfigWithPath(ctx, configFile)
	return cfg, err
}

func loadConfigWithPath(ctx context.Context, configFile string) (*config.Config, string, error) {
	var configPath string
	var err error

	if configFile != "" {
		configPath = configFile
	} else {
		configPath, err = config.FindConfigFile(".")
		if err != nil {
			return nil, "", oops.Wrapf(err, "find configuration file")
		}
	}

	cfg, err := config.LoadConfigWithIncludes(ctx, configPath)
	if err != nil {
		return nil, "", oops.Wrapf(err, "load configuration")
	}

	return cfg, configPath, nil
}

func saveConfig(cfg *config.Config, configPath string) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return oops.Wrapf(err, "marshal configuration")
	}

	backupPath := configPath + ".bak"
	if _, err := os.Stat(configPath); err == nil {
		input, err := os.ReadFile(configPath)
		if err != nil {
			return oops.Wrapf(err, "read original file for backup")
		}
		if err := os.WriteFile(backupPath, input, 0o644); err != nil {
			return oops.Wrapf(err, "create backup file")
		}
	}

	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		if _, statErr := os.Stat(backupPath); statErr == nil {
			utils.LogIfErr(os.Rename(backupPath, configPath))
		}
		return oops.Wrapf(err, "write configuration file")
	}

	utils.LogIfErr(os.Remove(backupPath))

	return nil
}

func setFieldValue(obj interface{}, fieldName string, value interface{}) error {
	v := reflect.ValueOf(obj).Elem()
	field := v.FieldByName(fieldName)
	if !field.IsValid() {
		return fmt.Errorf("field %s not found", fieldName)
	}
	if !field.CanSet() {
		return fmt.Errorf("field %s cannot be set", fieldName)
	}
	val := reflect.ValueOf(value)
	if field.Type() != val.Type() {
		return fmt.Errorf("type mismatch for field %s", fieldName)
	}
	field.Set(val)
	return nil
}
