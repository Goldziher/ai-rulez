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
	"github.com/Goldziher/ai-rulez/internal/errutils"
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
		rule, _ := element.(*config.Rule) //nolint:errcheck // Type assertion guaranteed by switch
		cfg.Rules = append(cfg.Rules, *rule)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Added rule: %s", rule.Name),
				},
			},
		}, nil
	case ElementTypeSections:
		section, _ := element.(*config.Section) //nolint:errcheck // Type assertion guaranteed by switch
		cfg.Sections = append(cfg.Sections, *section)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Added section: %s", section.Name),
				},
			},
		}, nil
	case ElementTypeAgents:
		agent, _ := element.(*config.Agent) //nolint:errcheck // Type assertion guaranteed by switch
		cfg.Agents = append(cfg.Agents, *agent)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Added agent: %s", agent.Name),
				},
			},
		}, nil
	case ElementTypeOutputs:
		output, _ := element.(*config.Output) //nolint:errcheck // Type assertion guaranteed by switch
		cfg.Outputs = append(cfg.Outputs, *output)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Added output: %s", output.Path),
				},
			},
		}, nil
	case ElementTypeCommands:
		cmd, _ := element.(*config.Command) //nolint:errcheck // Type assertion guaranteed by switch
		cfg.Commands = append(cfg.Commands, *cmd)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Added command: %s", cmd.Name),
				},
			},
		}, nil
	case ElementTypeMCPServers:
		server, _ := element.(*config.MCPServer) //nolint:errcheck // Type assertion guaranteed by switch
		cfg.MCPServers = append(cfg.MCPServers, *server)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Added MCP server: %s", server.Name),
				},
			},
		}, nil
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
		return handleGetRule(cfg, name)
	case ElementTypeSections:
		return handleGetSection(cfg, name)
	case ElementTypeAgents:
		return handleGetAgent(cfg, name)
	case ElementTypeOutputs:
		return handleGetOutput(cfg, name)
	case ElementTypeCommands:
		return handleGetCommand(cfg, name)
	case ElementTypeMCPServers:
		return handleGetMCPServer(cfg, name)
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
		configPath = "ai_rulez.yaml"
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

func handleGetRule(cfg *config.Config, name string) (*mcp.CallToolResult, error) {
	for _, rule := range cfg.Rules {
		if rule.Name != name {
			continue
		}
		var output strings.Builder
		output.WriteString(fmt.Sprintf("Name:     %s\n", rule.Name))
		if rule.ID != "" {
			output.WriteString(fmt.Sprintf("ID:       %s\n", rule.ID))
		}
		output.WriteString(fmt.Sprintf("Content:  %s\n", rule.Content))
		output.WriteString(fmt.Sprintf("Priority: %s\n", rule.Priority))
		if len(rule.Targets) > 0 {
			output.WriteString(fmt.Sprintf("Targets:  %v", rule.Targets))
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
	return nil, fmt.Errorf("rule not found: %s", name)
}

func handleGetSection(cfg *config.Config, name string) (*mcp.CallToolResult, error) {
	for _, section := range cfg.Sections {
		if section.Name != name {
			continue
		}
		var output strings.Builder
		output.WriteString(fmt.Sprintf("Name:     %s\n", section.Name))
		if section.ID != "" {
			output.WriteString(fmt.Sprintf("ID:       %s\n", section.ID))
		}
		output.WriteString(fmt.Sprintf("Content:  %s\n", section.Content))
		output.WriteString(fmt.Sprintf("Priority: %s\n", section.Priority))
		if len(section.Targets) > 0 {
			output.WriteString(fmt.Sprintf("Targets:  %v", section.Targets))
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
	return nil, fmt.Errorf("section not found: %s", name)
}

func handleGetAgent(cfg *config.Config, name string) (*mcp.CallToolResult, error) {
	for i := range cfg.Agents {
		if cfg.Agents[i].Name != name {
			continue
		}
		var output strings.Builder
		output.WriteString(fmt.Sprintf("Name:         %s\n", cfg.Agents[i].Name))
		if cfg.Agents[i].ID != "" {
			output.WriteString(fmt.Sprintf("ID:           %s\n", cfg.Agents[i].ID))
		}
		if cfg.Agents[i].Description != "" {
			output.WriteString(fmt.Sprintf("Description:   %s\n", cfg.Agents[i].Description))
		}
		output.WriteString(fmt.Sprintf("Priority:     %s\n", cfg.Agents[i].Priority))
		if len(cfg.Agents[i].Tools) > 0 {
			output.WriteString(fmt.Sprintf("Tools:        [%s]\n", strings.Join(cfg.Agents[i].Tools, " ")))
		}
		if cfg.Agents[i].Model != "" {
			output.WriteString(fmt.Sprintf("Model:        %s\n", cfg.Agents[i].Model))
		}
		if cfg.Agents[i].SystemPrompt != "" {
			output.WriteString(fmt.Sprintf("System Prompt: %s\n", cfg.Agents[i].SystemPrompt))
		}
		if cfg.Agents[i].Template != nil {
			switch t := cfg.Agents[i].Template.(type) {
			case string:
				output.WriteString(fmt.Sprintf("Template:     %s\n", t))
			case map[string]interface{}:
				if inline, ok := t["inline"].(string); ok {
					output.WriteString("Template Type: inline\n")
					output.WriteString(fmt.Sprintf("Template Value: %s\n", inline))
				} else if file, ok := t["file"].(string); ok {
					output.WriteString("Template Type: file\n")
					output.WriteString(fmt.Sprintf("Template Value: %s\n", file))
				} else {
					output.WriteString(fmt.Sprintf("Template:     %v\n", cfg.Agents[i].Template))
				}
			default:
				output.WriteString(fmt.Sprintf("Template:     %v\n", cfg.Agents[i].Template))
			}
		}
		if len(cfg.Agents[i].Targets) > 0 {
			output.WriteString(fmt.Sprintf("Targets:      [%s]", strings.Join(cfg.Agents[i].Targets, " ")))
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
	return nil, fmt.Errorf("agent not found: %s", name)
}

func handleGetOutput(cfg *config.Config, name string) (*mcp.CallToolResult, error) {
	for i := range cfg.Outputs {
		if cfg.Outputs[i].Path != name {
			continue
		}
		var outputStr strings.Builder
		outputStr.WriteString(fmt.Sprintf("Path:         %s\n", cfg.Outputs[i].Path))
		if cfg.Outputs[i].Type != "" {
			outputStr.WriteString(fmt.Sprintf("Type:         %s\n", cfg.Outputs[i].Type))
		}
		if cfg.Outputs[i].Template != nil {
			switch t := cfg.Outputs[i].Template.(type) {
			case string:
				outputStr.WriteString(fmt.Sprintf("Template:     %s\n", t))
			case map[string]interface{}:
				if templateType, ok := t["type"].(string); ok {
					if templateValue, ok := t["value"].(string); ok {
						outputStr.WriteString(fmt.Sprintf("Template Type: %s\n", templateType))
						outputStr.WriteString(fmt.Sprintf("Template Value: %s\n", templateValue))
					} else {
						outputStr.WriteString(fmt.Sprintf("Template Type: %s\n", templateType))
						outputStr.WriteString("Template Value: (invalid)\n")
					}
				} else {
					if inline, ok := t["inline"].(string); ok {
						outputStr.WriteString("Template Type: inline\n")
						outputStr.WriteString(fmt.Sprintf("Template Value: %s\n", inline))
					} else if file, ok := t["file"].(string); ok {
						outputStr.WriteString("Template Type: file\n")
						outputStr.WriteString(fmt.Sprintf("Template Value: %s\n", file))
					} else {
						outputStr.WriteString(fmt.Sprintf("Template:     %v\n", cfg.Outputs[i].Template))
					}
				}
			default:
				outputStr.WriteString(fmt.Sprintf("Template:     %v\n", cfg.Outputs[i].Template))
			}
		}
		if cfg.Outputs[i].NamingScheme != "" {
			outputStr.WriteString(fmt.Sprintf("Naming Scheme: %s", cfg.Outputs[i].NamingScheme))
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: strings.TrimSpace(outputStr.String()),
				},
			},
		}, nil
	}
	return nil, fmt.Errorf("output not found: %s", name)
}

func handleGetCommand(cfg *config.Config, name string) (*mcp.CallToolResult, error) {
	for i := range cfg.Commands {
		if cfg.Commands[i].Name != name {
			continue
		}
		var output strings.Builder
		output.WriteString(fmt.Sprintf("Name:         %s\n", cfg.Commands[i].Name))
		if cfg.Commands[i].ID != "" {
			output.WriteString(fmt.Sprintf("ID:           %s\n", cfg.Commands[i].ID))
		}
		if cfg.Commands[i].Description != "" {
			output.WriteString(fmt.Sprintf("Description:  %s\n", cfg.Commands[i].Description))
		}
		if len(cfg.Commands[i].Aliases) > 0 {
			output.WriteString(fmt.Sprintf("Aliases:      [%s]\n", strings.Join(cfg.Commands[i].Aliases, " ")))
		}
		if cfg.Commands[i].Usage != "" {
			output.WriteString(fmt.Sprintf("Usage:        %s\n", cfg.Commands[i].Usage))
		}
		if cfg.Commands[i].SystemPrompt != "" {
			output.WriteString(fmt.Sprintf("System Prompt: %s\n", cfg.Commands[i].SystemPrompt))
		}
		if cfg.Commands[i].Shortcut != "" {
			output.WriteString(fmt.Sprintf("Shortcut:     %s\n", cfg.Commands[i].Shortcut))
		}
		if cfg.Commands[i].Enabled != nil {
			output.WriteString(fmt.Sprintf("Enabled:      %t\n", *cfg.Commands[i].Enabled))
		}
		if len(cfg.Commands[i].Targets) > 0 {
			output.WriteString(fmt.Sprintf("Targets:      [%s]", strings.Join(cfg.Commands[i].Targets, " ")))
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
	return nil, fmt.Errorf("command not found: %s", name)
}

func handleGetMCPServer(cfg *config.Config, name string) (*mcp.CallToolResult, error) {
	for i := range cfg.MCPServers {
		if cfg.MCPServers[i].Name != name {
			continue
		}
		var output strings.Builder
		output.WriteString(fmt.Sprintf("Name:         %s\n", cfg.MCPServers[i].Name))
		if cfg.MCPServers[i].ID != "" {
			output.WriteString(fmt.Sprintf("ID:           %s\n", cfg.MCPServers[i].ID))
		}
		if cfg.MCPServers[i].Description != "" {
			output.WriteString(fmt.Sprintf("Description:  %s\n", cfg.MCPServers[i].Description))
		}
		if cfg.MCPServers[i].Command != "" {
			output.WriteString(fmt.Sprintf("Command:      %s\n", cfg.MCPServers[i].Command))
		}
		if len(cfg.MCPServers[i].Args) > 0 {
			output.WriteString(fmt.Sprintf("Args:         [%s]\n", strings.Join(cfg.MCPServers[i].Args, " ")))
		}
		if len(cfg.MCPServers[i].Env) > 0 {
			var envStrs []string
			for k, v := range cfg.MCPServers[i].Env {
				envStrs = append(envStrs, fmt.Sprintf("%s=%s", k, v))
			}
			output.WriteString(fmt.Sprintf("Env:          [%s]\n", strings.Join(envStrs, " ")))
		}
		if cfg.MCPServers[i].Transport != "" {
			output.WriteString(fmt.Sprintf("Transport:    %s\n", cfg.MCPServers[i].Transport))
		}
		if cfg.MCPServers[i].URL != "" {
			output.WriteString(fmt.Sprintf("URL:          %s\n", cfg.MCPServers[i].URL))
		}
		if cfg.MCPServers[i].Enabled != nil {
			output.WriteString(fmt.Sprintf("Enabled:      %t\n", *cfg.MCPServers[i].Enabled))
		}
		if len(cfg.MCPServers[i].Targets) > 0 {
			output.WriteString(fmt.Sprintf("Targets:      [%s]", strings.Join(cfg.MCPServers[i].Targets, " ")))
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
	return nil, fmt.Errorf("MCP server not found: %s", name)
}

func HandleList(ctx context.Context, elementType string, cfg *config.Config) (*mcp.CallToolResult, error) {
	switch elementType {
	case ElementTypeRules:
		return handleListRules(cfg)
	case ElementTypeSections:
		return handleListSections(cfg)
	case ElementTypeAgents:
		return handleListAgents(cfg)
	case ElementTypeOutputs:
		return handleListOutputs(cfg)
	case ElementTypeCommands:
		return handleListCommands(cfg)
	case ElementTypeMCPServers:
		return handleListMCPServers(cfg)
	case ElementTypeIncludes:
		return handleListIncludes(cfg)
	default:
		return nil, fmt.Errorf("unsupported element type: %s", elementType)
	}
}

func handleListRules(cfg *config.Config) (*mcp.CallToolResult, error) {
	var items []string
	if len(cfg.Rules) == 0 {
		items = append(items, "No rules found")
	} else {
		for _, rule := range cfg.Rules {
			desc := fmt.Sprintf("Name: %s, Priority: %s", rule.Name, rule.Priority)
			if len(rule.Targets) > 0 {
				desc += fmt.Sprintf(", Targets: [%s]", strings.Join(rule.Targets, " "))
			}
			items = append(items, fmt.Sprintf("  %s", desc))
		}
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: fmt.Sprintf("Rules (%d):\n%s", len(cfg.Rules), strings.Join(items, "\n")),
			},
		},
	}, nil
}

func handleListSections(cfg *config.Config) (*mcp.CallToolResult, error) {
	var items []string
	if len(cfg.Sections) == 0 {
		items = append(items, "No sections found")
	} else {
		for _, section := range cfg.Sections {
			desc := fmt.Sprintf("Name: %s, Priority: %s", section.Name, section.Priority)
			if len(section.Targets) > 0 {
				desc += fmt.Sprintf(", Targets: [%s]", strings.Join(section.Targets, " "))
			}
			items = append(items, fmt.Sprintf("  %s", desc))
		}
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: fmt.Sprintf("Sections (%d):\n%s", len(cfg.Sections), strings.Join(items, "\n")),
			},
		},
	}, nil
}

func handleListAgents(cfg *config.Config) (*mcp.CallToolResult, error) {
	var items []string
	if len(cfg.Agents) == 0 {
		items = append(items, "No agents found")
	} else {
		for i := range cfg.Agents {
			desc := fmt.Sprintf("Name: %s", cfg.Agents[i].Name)
			if cfg.Agents[i].Description != "" {
				desc += fmt.Sprintf(", Description: %s", cfg.Agents[i].Description)
			}
			if len(cfg.Agents[i].Tools) > 0 {
				desc += fmt.Sprintf(", Tools: [%s]", strings.Join(cfg.Agents[i].Tools, " "))
			}
			items = append(items, fmt.Sprintf("  %s", desc))
		}
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: fmt.Sprintf("Agents (%d):\n%s", len(cfg.Agents), strings.Join(items, "\n")),
			},
		},
	}, nil
}

func handleListOutputs(cfg *config.Config) (*mcp.CallToolResult, error) {
	var items []string
	if len(cfg.Outputs) == 0 {
		items = append(items, "No outputs found")
	} else {
		for i := range cfg.Outputs {
			desc := fmt.Sprintf("Path: %s", cfg.Outputs[i].Path)
			if cfg.Outputs[i].Type != "" {
				desc += fmt.Sprintf(", Type: %s", cfg.Outputs[i].Type)
			}
			items = append(items, fmt.Sprintf("  %s", desc))
		}
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: fmt.Sprintf("Outputs (%d):\n%s", len(cfg.Outputs), strings.Join(items, "\n")),
			},
		},
	}, nil
}

func handleListCommands(cfg *config.Config) (*mcp.CallToolResult, error) {
	var items []string
	if len(cfg.Commands) == 0 {
		items = append(items, "No commands found")
	} else {
		for i := range cfg.Commands {
			desc := fmt.Sprintf("Name: %s", cfg.Commands[i].Name)
			if cfg.Commands[i].Description != "" {
				desc += fmt.Sprintf(", Description: %s", cfg.Commands[i].Description)
			}
			if len(cfg.Commands[i].Aliases) > 0 {
				desc += fmt.Sprintf(", Aliases: [%s]", strings.Join(cfg.Commands[i].Aliases, " "))
			}
			items = append(items, fmt.Sprintf("  %s", desc))
		}
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: fmt.Sprintf("Commands (%d):\n%s", len(cfg.Commands), strings.Join(items, "\n")),
			},
		},
	}, nil
}

func handleListMCPServers(cfg *config.Config) (*mcp.CallToolResult, error) {
	var items []string
	if len(cfg.MCPServers) == 0 {
		items = append(items, "No MCP servers found")
	} else {
		for i := range cfg.MCPServers {
			desc := fmt.Sprintf("Name: %s", cfg.MCPServers[i].Name)
			if cfg.MCPServers[i].Description != "" {
				desc += fmt.Sprintf(", Description: %s", cfg.MCPServers[i].Description)
			}
			if cfg.MCPServers[i].Command != "" {
				desc += fmt.Sprintf(", Command: %s", cfg.MCPServers[i].Command)
			}
			items = append(items, fmt.Sprintf("  %s", desc))
		}
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: fmt.Sprintf("MCP Servers (%d):\n%s", len(cfg.MCPServers), strings.Join(items, "\n")),
			},
		},
	}, nil
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
		return handleUpdateRule(name, updates, cfg)
	case ElementTypeSections:
		return handleUpdateSection(name, updates, cfg)
	case ElementTypeAgents:
		return handleUpdateAgent(name, updates, cfg)
	case ElementTypeOutputs:
		return handleUpdateOutput(name, updates, cfg)
	case ElementTypeCommands:
		return handleUpdateCommand(name, updates, cfg)
	case ElementTypeMCPServers:
		return handleUpdateMCPServer(name, updates, cfg)
	default:
		return nil, fmt.Errorf("unsupported element type: %s", elementType)
	}
}

func handleUpdateMetadata(updates map[string]interface{}, cfg *config.Config) (*mcp.CallToolResult, error) {
	for key, value := range updates {
		errutils.LogIfErr(setFieldValue(&cfg.Metadata, key, value))
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

func handleUpdateRule(name string, updates map[string]interface{}, cfg *config.Config) (*mcp.CallToolResult, error) {
	for i, rule := range cfg.Rules {
		if rule.Name == name {
			for key, value := range updates {
				errutils.LogIfErr(setFieldValue(&cfg.Rules[i], key, value))
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{
						Type: "text",
						Text: fmt.Sprintf("Updated rule: %s", name),
					},
				},
			}, nil
		}
	}
	return nil, fmt.Errorf("rule not found: %s", name)
}

func handleUpdateSection(name string, updates map[string]interface{}, cfg *config.Config) (*mcp.CallToolResult, error) {
	for i, section := range cfg.Sections {
		if section.Name == name {
			for key, value := range updates {
				errutils.LogIfErr(setFieldValue(&cfg.Sections[i], key, value))
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{
						Type: "text",
						Text: fmt.Sprintf("Updated section: %s", name),
					},
				},
			}, nil
		}
	}
	return nil, fmt.Errorf("section not found: %s", name)
}

func handleUpdateAgent(name string, updates map[string]interface{}, cfg *config.Config) (*mcp.CallToolResult, error) {
	for i := range cfg.Agents {
		if cfg.Agents[i].Name == name {
			for key, value := range updates {
				errutils.LogIfErr(setFieldValue(&cfg.Agents[i], key, value))
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{
						Type: "text",
						Text: fmt.Sprintf("Updated agent: %s", name),
					},
				},
			}, nil
		}
	}
	return nil, fmt.Errorf("agent not found: %s", name)
}

func handleUpdateOutput(name string, updates map[string]interface{}, cfg *config.Config) (*mcp.CallToolResult, error) {
	for i, output := range cfg.Outputs {
		if output.Path == name {
			for key, value := range updates {
				errutils.LogIfErr(setFieldValue(&cfg.Outputs[i], key, value))
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{
						Type: "text",
						Text: fmt.Sprintf("Updated output: %s", name),
					},
				},
			}, nil
		}
	}
	return nil, fmt.Errorf("output not found: %s", name)
}

func handleUpdateCommand(name string, updates map[string]interface{}, cfg *config.Config) (*mcp.CallToolResult, error) {
	for i := range cfg.Commands {
		if cfg.Commands[i].Name == name {
			for key, value := range updates {
				errutils.LogIfErr(setFieldValue(&cfg.Commands[i], key, value))
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{
						Type: "text",
						Text: fmt.Sprintf("Updated command: %s", name),
					},
				},
			}, nil
		}
	}
	return nil, fmt.Errorf("command not found: %s", name)
}

func handleUpdateMCPServer(name string, updates map[string]interface{}, cfg *config.Config) (*mcp.CallToolResult, error) {
	for i := range cfg.MCPServers {
		if cfg.MCPServers[i].Name == name {
			for key, value := range updates {
				errutils.LogIfErr(setFieldValue(&cfg.MCPServers[i], key, value))
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{
						Type: "text",
						Text: fmt.Sprintf("Updated MCP server: %s", name),
					},
				},
			}, nil
		}
	}
	return nil, fmt.Errorf("MCP server not found: %s", name)
}

func HandleDelete(ctx context.Context, elementType, name string, cfg *config.Config) (*mcp.CallToolResult, error) {
	switch elementType {
	case ElementTypeExtends:
		return handleDeleteExtends(cfg)
	case ElementTypeRules:
		return handleDeleteRule(name, cfg)
	case ElementTypeSections:
		return handleDeleteSection(name, cfg)
	case ElementTypeAgents:
		return handleDeleteAgent(name, cfg)
	case ElementTypeOutputs:
		return handleDeleteOutput(name, cfg)
	case ElementTypeCommands:
		return handleDeleteCommand(name, cfg)
	case ElementTypeMCPServers:
		return handleDeleteMCPServer(name, cfg)
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

func handleDeleteRule(name string, cfg *config.Config) (*mcp.CallToolResult, error) {
	for i, rule := range cfg.Rules {
		if rule.Name == name {
			cfg.Rules = append(cfg.Rules[:i], cfg.Rules[i+1:]...)
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{
						Type: "text",
						Text: fmt.Sprintf("Deleted rule: %s", name),
					},
				},
			}, nil
		}
	}
	return nil, fmt.Errorf("rule not found: %s", name)
}

func handleDeleteSection(name string, cfg *config.Config) (*mcp.CallToolResult, error) {
	for i, section := range cfg.Sections {
		if section.Name == name {
			cfg.Sections = append(cfg.Sections[:i], cfg.Sections[i+1:]...)
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{
						Type: "text",
						Text: fmt.Sprintf("Deleted section: %s", name),
					},
				},
			}, nil
		}
	}
	return nil, fmt.Errorf("section not found: %s", name)
}

func handleDeleteAgent(name string, cfg *config.Config) (*mcp.CallToolResult, error) {
	for i := range cfg.Agents {
		if cfg.Agents[i].Name == name {
			cfg.Agents = append(cfg.Agents[:i], cfg.Agents[i+1:]...)
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{
						Type: "text",
						Text: fmt.Sprintf("Deleted agent: %s", name),
					},
				},
			}, nil
		}
	}
	return nil, fmt.Errorf("agent not found: %s", name)
}

func handleDeleteOutput(name string, cfg *config.Config) (*mcp.CallToolResult, error) {
	for i, output := range cfg.Outputs {
		if output.Path == name {
			cfg.Outputs = append(cfg.Outputs[:i], cfg.Outputs[i+1:]...)
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{
						Type: "text",
						Text: fmt.Sprintf("Deleted output: %s", name),
					},
				},
			}, nil
		}
	}
	return nil, fmt.Errorf("output not found: %s", name)
}

func handleDeleteCommand(name string, cfg *config.Config) (*mcp.CallToolResult, error) {
	for i := range cfg.Commands {
		if cfg.Commands[i].Name == name {
			cfg.Commands = append(cfg.Commands[:i], cfg.Commands[i+1:]...)
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{
						Type: "text",
						Text: fmt.Sprintf("Deleted command: %s", name),
					},
				},
			}, nil
		}
	}
	return nil, fmt.Errorf("command not found: %s", name)
}

func handleDeleteMCPServer(name string, cfg *config.Config) (*mcp.CallToolResult, error) {
	for i := range cfg.MCPServers {
		if cfg.MCPServers[i].Name == name {
			cfg.MCPServers = append(cfg.MCPServers[:i], cfg.MCPServers[i+1:]...)
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{
						Type: "text",
						Text: fmt.Sprintf("Deleted MCP server: %s", name),
					},
				},
			}, nil
		}
	}
	return nil, fmt.Errorf("MCP server not found: %s", name)
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
	var responseData map[string]interface{}

	switch elementType {
	case ElementTypeRules:
		responseData = map[string]interface{}{
			"rules": cfg.Rules,
		}
	case ElementTypeSections:
		responseData = map[string]interface{}{
			"sections": cfg.Sections,
		}
	case ElementTypeAgents:
		responseData = map[string]interface{}{
			"agents": cfg.Agents,
		}
	case ElementTypeOutputs:
		responseData = map[string]interface{}{
			"outputs": cfg.Outputs,
		}
	case ElementTypeMCPServers:
		responseData = map[string]interface{}{
			"mcp_servers": cfg.MCPServers,
		}
	case ElementTypeCommands:
		responseData = map[string]interface{}{
			"commands": cfg.Commands,
		}
	default:
		return nil, fmt.Errorf("unknown element type: %s", elementType)
	}

	jsonData, err := json.Marshal(responseData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response: %w", err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: string(jsonData),
			},
		},
	}, nil
}

func HandleListJSONWithFilter(ctx context.Context, elementType string, cfg *config.Config, nameFilter string) (*mcp.CallToolResult, error) {
	var responseData map[string]interface{}
	var err error

	switch elementType {
	case ElementTypeRules:
		responseData, err = handleListJSONFilteredRules(cfg, nameFilter)
	case ElementTypeSections:
		responseData, err = handleListJSONFilteredSections(cfg, nameFilter)
	case ElementTypeAgents:
		responseData, err = handleListJSONFilteredAgents(cfg, nameFilter)
	case ElementTypeOutputs:
		responseData, err = handleListJSONFilteredOutputs(cfg, nameFilter)
	default:
		return nil, fmt.Errorf("unknown element type: %s", elementType)
	}

	if err != nil {
		return nil, err
	}

	jsonData, err := json.Marshal(responseData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response: %w", err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: string(jsonData),
			},
		},
	}, nil
}

func handleListJSONFilteredRules(cfg *config.Config, nameFilter string) (map[string]interface{}, error) {
	var filteredRules []config.Rule
	for _, rule := range cfg.Rules {
		if nameFilter == "" || strings.Contains(strings.ToLower(rule.Name), strings.ToLower(nameFilter)) {
			filteredRules = append(filteredRules, rule)
		}
	}
	return map[string]interface{}{
		"rules": filteredRules,
	}, nil
}

func handleListJSONFilteredSections(cfg *config.Config, nameFilter string) (map[string]interface{}, error) {
	var filteredSections []config.Section
	for _, section := range cfg.Sections {
		if nameFilter == "" || strings.Contains(strings.ToLower(section.Name), strings.ToLower(nameFilter)) {
			filteredSections = append(filteredSections, section)
		}
	}
	return map[string]interface{}{
		"sections": filteredSections,
	}, nil
}

func handleListJSONFilteredAgents(cfg *config.Config, nameFilter string) (map[string]interface{}, error) {
	var filteredAgents []config.Agent
	for i := range cfg.Agents {
		if nameFilter == "" || strings.Contains(strings.ToLower(cfg.Agents[i].Name), strings.ToLower(nameFilter)) {
			filteredAgents = append(filteredAgents, cfg.Agents[i])
		}
	}
	return map[string]interface{}{
		"agents": filteredAgents,
	}, nil
}

func handleListJSONFilteredOutputs(cfg *config.Config, nameFilter string) (map[string]interface{}, error) {
	var filteredOutputs []config.Output
	for _, output := range cfg.Outputs {
		if nameFilter == "" || strings.Contains(strings.ToLower(output.Path), strings.ToLower(nameFilter)) {
			filteredOutputs = append(filteredOutputs, output)
		}
	}
	return map[string]interface{}{
		"outputs": filteredOutputs,
	}, nil
}

func HandleGetJSON(ctx context.Context, elementType, name string, cfg *config.Config) (*mcp.CallToolResult, error) {
	var responseData map[string]interface{}
	var err error

	switch elementType {
	case ElementTypeRules:
		responseData, err = handleGetJSONRule(cfg, name)
	case ElementTypeSections:
		responseData, err = handleGetJSONSection(cfg, name)
	case ElementTypeAgents:
		responseData, err = handleGetJSONAgent(cfg, name)
	case ElementTypeOutputs:
		responseData, err = handleGetJSONOutput(cfg, name)
	case ElementTypeMCPServers:
		responseData, err = handleGetJSONMCPServer(cfg, name)
	case ElementTypeCommands:
		responseData, err = handleGetJSONCommand(cfg, name)
	default:
		return nil, fmt.Errorf("unknown element type: %s", elementType)
	}

	if err != nil {
		return nil, err
	}

	if responseData == nil {
		return nil, fmt.Errorf("%s not found: %s", elementType, name)
	}

	jsonData, err := json.Marshal(responseData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response: %w", err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: string(jsonData),
			},
		},
	}, nil
}

func handleGetJSONRule(cfg *config.Config, name string) (map[string]interface{}, error) {
	for _, rule := range cfg.Rules {
		if rule.Name == name {
			return map[string]interface{}{
				"rule": rule,
			}, nil
		}
	}
	return nil, nil
}

func handleGetJSONSection(cfg *config.Config, name string) (map[string]interface{}, error) {
	for _, section := range cfg.Sections {
		if section.Name == name {
			return map[string]interface{}{
				"section": section,
			}, nil
		}
	}
	return nil, nil
}

func handleGetJSONAgent(cfg *config.Config, name string) (map[string]interface{}, error) {
	for i := range cfg.Agents {
		if cfg.Agents[i].Name == name {
			return map[string]interface{}{
				"agent": cfg.Agents[i],
			}, nil
		}
	}
	return nil, nil
}

func handleGetJSONOutput(cfg *config.Config, name string) (map[string]interface{}, error) {
	for i := range cfg.Outputs {
		if cfg.Outputs[i].Path == name {
			return map[string]interface{}{
				"output": cfg.Outputs[i],
			}, nil
		}
	}
	return nil, nil
}

func handleGetJSONMCPServer(cfg *config.Config, name string) (map[string]interface{}, error) {
	for i := range cfg.MCPServers {
		if cfg.MCPServers[i].Name == name {
			return map[string]interface{}{
				"mcp_server": cfg.MCPServers[i],
			}, nil
		}
	}
	return nil, nil
}

func handleGetJSONCommand(cfg *config.Config, name string) (map[string]interface{}, error) {
	for i := range cfg.Commands {
		if cfg.Commands[i].Name == name {
			return map[string]interface{}{
				"command": cfg.Commands[i],
			}, nil
		}
	}
	return nil, nil
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

	found := false
	switch elementType {
	case ElementTypeRules:
		found = handleUpdateMCPRule(cfg, name, updates)
	case ElementTypeSections:
		found = handleUpdateMCPSection(cfg, name, updates)
	case ElementTypeAgents:
		found = handleUpdateMCPAgent(cfg, name, updates)
	case ElementTypeOutputs:
		found = handleUpdateMCPOutput(cfg, name, updates)
	case ElementTypeMCPServers:
		found = handleUpdateMCPMCPServer(cfg, name, updates)
	case ElementTypeCommands:
		found = handleUpdateMCPCommand(cfg, name, updates)
	default:
		return nil, fmt.Errorf("unknown element type: %s", elementType)
	}

	if !found {
		return nil, fmt.Errorf("%s not found: %s", elementType, name)
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

func handleUpdateMCPRule(cfg *config.Config, name string, updates map[string]interface{}) bool {
	for i, rule := range cfg.Rules {
		if rule.Name == name {
			for key, value := range updates {
				errutils.LogIfErr(setFieldValue(&cfg.Rules[i], key, value))
			}
			return true
		}
	}
	return false
}

func handleUpdateMCPSection(cfg *config.Config, name string, updates map[string]interface{}) bool {
	for i, section := range cfg.Sections {
		if section.Name == name {
			for key, value := range updates {
				errutils.LogIfErr(setFieldValue(&cfg.Sections[i], key, value))
			}
			return true
		}
	}
	return false
}

func handleUpdateMCPAgent(cfg *config.Config, name string, updates map[string]interface{}) bool {
	for i := range cfg.Agents {
		if cfg.Agents[i].Name == name {
			for key, value := range updates {
				errutils.LogIfErr(setFieldValue(&cfg.Agents[i], key, value))
			}
			return true
		}
	}
	return false
}

func handleUpdateMCPOutput(cfg *config.Config, name string, updates map[string]interface{}) bool {
	for i, output := range cfg.Outputs {
		if output.Path == name {
			for key, value := range updates {
				errutils.LogIfErr(setFieldValue(&cfg.Outputs[i], key, value))
			}
			return true
		}
	}
	return false
}

func handleUpdateMCPMCPServer(cfg *config.Config, name string, updates map[string]interface{}) bool {
	for i := range cfg.MCPServers {
		if cfg.MCPServers[i].Name == name {
			for key, value := range updates {
				errutils.LogIfErr(setFieldValue(&cfg.MCPServers[i], key, value))
			}
			return true
		}
	}
	return false
}

func handleUpdateMCPCommand(cfg *config.Config, name string, updates map[string]interface{}) bool {
	for i := range cfg.Commands {
		if cfg.Commands[i].Name == name {
			for key, value := range updates {
				errutils.LogIfErr(setFieldValue(&cfg.Commands[i], key, value))
			}
			return true
		}
	}
	return false
}

func HandleDeleteMCP(ctx context.Context, elementType, name string) (*mcp.CallToolResult, error) {
	cfg, configPath, err := loadConfigWithPath(ctx, "")
	if err != nil {
		return nil, err
	}

	found := false
	switch elementType {
	case ElementTypeRules:
		found = handleDeleteMCPRule(cfg, name)
	case ElementTypeSections:
		found = handleDeleteMCPSection(cfg, name)
	case ElementTypeAgents:
		found = handleDeleteMCPAgent(cfg, name)
	case ElementTypeOutputs:
		found = handleDeleteMCPOutput(cfg, name)
	case ElementTypeMCPServers:
		found = handleDeleteMCPMCPServer(cfg, name)
	case ElementTypeCommands:
		found = handleDeleteMCPCommand(cfg, name)
	default:
		return nil, fmt.Errorf("unknown element type: %s", elementType)
	}

	if !found {
		return nil, fmt.Errorf("%s not found: %s", elementType, name)
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

func handleDeleteMCPRule(cfg *config.Config, name string) bool {
	for i, rule := range cfg.Rules {
		if rule.Name == name {
			cfg.Rules = append(cfg.Rules[:i], cfg.Rules[i+1:]...)
			return true
		}
	}
	return false
}

func handleDeleteMCPSection(cfg *config.Config, name string) bool {
	for i, section := range cfg.Sections {
		if section.Name == name {
			cfg.Sections = append(cfg.Sections[:i], cfg.Sections[i+1:]...)
			return true
		}
	}
	return false
}

func handleDeleteMCPAgent(cfg *config.Config, name string) bool {
	for i := range cfg.Agents {
		if cfg.Agents[i].Name == name {
			cfg.Agents = append(cfg.Agents[:i], cfg.Agents[i+1:]...)
			return true
		}
	}
	return false
}

func handleDeleteMCPOutput(cfg *config.Config, name string) bool {
	for i, output := range cfg.Outputs {
		if output.Path == name {
			cfg.Outputs = append(cfg.Outputs[:i], cfg.Outputs[i+1:]...)
			return true
		}
	}
	return false
}

func handleDeleteMCPMCPServer(cfg *config.Config, name string) bool {
	for i := range cfg.MCPServers {
		if cfg.MCPServers[i].Name == name {
			cfg.MCPServers = append(cfg.MCPServers[:i], cfg.MCPServers[i+1:]...)
			return true
		}
	}
	return false
}

func handleDeleteMCPCommand(cfg *config.Config, name string) bool {
	for i := range cfg.Commands {
		if cfg.Commands[i].Name == name {
			cfg.Commands = append(cfg.Commands[:i], cfg.Commands[i+1:]...)
			return true
		}
	}
	return false
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

func InterfaceSliceToStringSlice(arr []interface{}) []string {
	result := make([]string, len(arr))
	for i, v := range arr {
		result[i] = fmt.Sprintf("%v", v)
	}
	return result
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
			errutils.LogIfErr(os.Rename(backupPath, configPath))
		}
		return oops.Wrapf(err, "write configuration file")
	}

	errutils.LogIfErr(os.Remove(backupPath))

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
