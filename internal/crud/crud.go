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

// Helper function to get singular form of element type
func getSingular(elementType string) string {
	switch elementType {
	case "rules":
		return "rule"
	case "sections":
		return "section"
	case "agents":
		return "agent"
	case "outputs":
		return "output"
	case "commands":
		return "command"
	case "mcp_servers":
		return "MCP server"
	case "includes":
		return "include"
	case "extends":
		return "extends"
	case "metadata":
		return "metadata"
	default:
		return strings.TrimSuffix(elementType, "s")
	}
}

// formatConfigError formats configuration errors with detailed validation information
func formatConfigError(err error) string {
	if oopsErr, ok := oops.AsOops(err); ok {
		var details strings.Builder
		details.WriteString(err.Error())

		// Check if there are validation errors stored in the oops error
		context := oopsErr.Context()
		if errors, exists := context["errors"]; exists {
			details.WriteString("\n\nValidation errors:")
			if errorList, ok := errors.([]string); ok {
				for _, validationErr := range errorList {
					details.WriteString("\n")
					details.WriteString(validationErr)
				}
			}
		}

		// Add hint if available
		if hint := oopsErr.Hint(); hint != "" {
			details.WriteString("\n\nHint: ")
			details.WriteString(hint)
		}

		return details.String()
	}
	return err.Error()
}

// CLI helper functions for direct use from CLI commands
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

	// Print the result content
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

	// For includes, use raw config loading to avoid processing includes
	if elementType == "includes" {
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

	// Print the result content
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

	// For includes, use raw config loading to avoid processing includes
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

	// For includes, use raw config loading to avoid processing includes
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

// MCP handler functions
func HandleAdd(ctx context.Context, elementType string, element interface{}, cfg *config.Config) (*mcp.CallToolResult, error) {
	switch elementType {
	case "rules":
		rule := element.(*config.Rule) // Type assertion guaranteed by switch
		cfg.Rules = append(cfg.Rules, *rule)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Added rule: %s", rule.Name),
				},
			},
		}, nil
	case "sections":
		section := element.(*config.Section) // Type assertion guaranteed by switch
		cfg.Sections = append(cfg.Sections, *section)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Added section: %s", section.Name),
				},
			},
		}, nil
	case "agents":
		agent := element.(*config.Agent) // Type assertion guaranteed by switch
		cfg.Agents = append(cfg.Agents, *agent)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Added agent: %s", agent.Name),
				},
			},
		}, nil
	case "outputs":
		output := element.(*config.Output) // Type assertion guaranteed by switch
		cfg.Outputs = append(cfg.Outputs, *output)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Added output: %s", output.Path),
				},
			},
		}, nil
	case "commands":
		cmd := element.(*config.Command) // Type assertion guaranteed by switch
		cfg.Commands = append(cfg.Commands, *cmd)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Added command: %s", cmd.Name),
				},
			},
		}, nil
	case "mcp_servers":
		server := element.(*config.MCPServer) // Type assertion guaranteed by switch
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
	case "metadata":
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
	case "extends":
		// For extends, we need to load the raw config to get the original extends value
		// because the merged config doesn't preserve the extends field
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
			}, nil
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
	case "rules":
		for _, rule := range cfg.Rules {
			if rule.Name == name {
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
		}
		return nil, fmt.Errorf("rule not found: %s", name)
	case "sections":
		for _, section := range cfg.Sections {
			if section.Name == name {
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
		}
		return nil, fmt.Errorf("section not found: %s", name)
	case "agents":
		for _, agent := range cfg.Agents {
			if agent.Name == name {
				var output strings.Builder
				output.WriteString(fmt.Sprintf("Name:         %s\n", agent.Name))
				if agent.ID != "" {
					output.WriteString(fmt.Sprintf("ID:           %s\n", agent.ID))
				}
				if agent.Description != "" {
					output.WriteString(fmt.Sprintf("Description:   %s\n", agent.Description))
				}
				output.WriteString(fmt.Sprintf("Priority:     %s\n", agent.Priority))
				if len(agent.Tools) > 0 {
					output.WriteString(fmt.Sprintf("Tools:        [%s]\n", strings.Join(agent.Tools, " ")))
				}
				if agent.SystemPrompt != "" {
					output.WriteString(fmt.Sprintf("System Prompt: %s\n", agent.SystemPrompt))
				}
				if agent.Template != nil {
					// Handle template display based on type
					switch t := agent.Template.(type) {
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
							output.WriteString(fmt.Sprintf("Template:     %v\n", agent.Template))
						}
					default:
						output.WriteString(fmt.Sprintf("Template:     %v\n", agent.Template))
					}
				}
				if len(agent.Targets) > 0 {
					output.WriteString(fmt.Sprintf("Targets:      [%s]", strings.Join(agent.Targets, " ")))
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
		}
		return nil, fmt.Errorf("agent not found: %s", name)
	case "outputs":
		for _, output := range cfg.Outputs {
			if output.Path == name {
				var outputStr strings.Builder
				outputStr.WriteString(fmt.Sprintf("Path:         %s\n", output.Path))
				if output.Type != "" {
					outputStr.WriteString(fmt.Sprintf("Type:         %s\n", output.Type))
				}
				if output.Template != nil {
					// Handle template display based on type
					switch t := output.Template.(type) {
					case string:
						outputStr.WriteString(fmt.Sprintf("Template:     %s\n", t))
					case map[string]interface{}:
						// Handle new structured template format
						if templateType, ok := t["type"].(string); ok {
							if templateValue, ok := t["value"].(string); ok {
								outputStr.WriteString(fmt.Sprintf("Template Type: %s\n", templateType))
								outputStr.WriteString(fmt.Sprintf("Template Value: %s\n", templateValue))
							} else {
								outputStr.WriteString(fmt.Sprintf("Template Type: %s\n", templateType))
								outputStr.WriteString("Template Value: (invalid)\n")
							}
						} else {
							// Fallback for legacy formats
							if inline, ok := t["inline"].(string); ok {
								outputStr.WriteString("Template Type: inline\n")
								outputStr.WriteString(fmt.Sprintf("Template Value: %s\n", inline))
							} else if file, ok := t["file"].(string); ok {
								outputStr.WriteString("Template Type: file\n")
								outputStr.WriteString(fmt.Sprintf("Template Value: %s\n", file))
							} else {
								outputStr.WriteString(fmt.Sprintf("Template:     %v\n", output.Template))
							}
						}
					default:
						outputStr.WriteString(fmt.Sprintf("Template:     %v\n", output.Template))
					}
				}
				if output.NamingScheme != "" {
					outputStr.WriteString(fmt.Sprintf("Naming Scheme: %s", output.NamingScheme))
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
		}
		return nil, fmt.Errorf("output not found: %s", name)
	case "commands":
		for _, cmd := range cfg.Commands {
			if cmd.Name == name {
				var output strings.Builder
				output.WriteString(fmt.Sprintf("Name:         %s\n", cmd.Name))
				if cmd.ID != "" {
					output.WriteString(fmt.Sprintf("ID:           %s\n", cmd.ID))
				}
				if cmd.Description != "" {
					output.WriteString(fmt.Sprintf("Description:  %s\n", cmd.Description))
				}
				if len(cmd.Aliases) > 0 {
					output.WriteString(fmt.Sprintf("Aliases:      [%s]\n", strings.Join(cmd.Aliases, " ")))
				}
				if cmd.Usage != "" {
					output.WriteString(fmt.Sprintf("Usage:        %s\n", cmd.Usage))
				}
				if cmd.SystemPrompt != "" {
					output.WriteString(fmt.Sprintf("System Prompt: %s\n", cmd.SystemPrompt))
				}
				if cmd.Shortcut != "" {
					output.WriteString(fmt.Sprintf("Shortcut:     %s\n", cmd.Shortcut))
				}
				if cmd.Enabled != nil {
					output.WriteString(fmt.Sprintf("Enabled:      %t\n", *cmd.Enabled))
				}
				if len(cmd.Targets) > 0 {
					output.WriteString(fmt.Sprintf("Targets:      [%s]", strings.Join(cmd.Targets, " ")))
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
		}
		return nil, fmt.Errorf("command not found: %s", name)
	case "mcp_servers":
		for _, server := range cfg.MCPServers {
			if server.Name == name {
				var output strings.Builder
				output.WriteString(fmt.Sprintf("Name:         %s\n", server.Name))
				if server.ID != "" {
					output.WriteString(fmt.Sprintf("ID:           %s\n", server.ID))
				}
				if server.Description != "" {
					output.WriteString(fmt.Sprintf("Description:  %s\n", server.Description))
				}
				if server.Command != "" {
					output.WriteString(fmt.Sprintf("Command:      %s\n", server.Command))
				}
				if len(server.Args) > 0 {
					output.WriteString(fmt.Sprintf("Args:         [%s]\n", strings.Join(server.Args, " ")))
				}
				if len(server.Env) > 0 {
					var envStrs []string
					for k, v := range server.Env {
						envStrs = append(envStrs, fmt.Sprintf("%s=%s", k, v))
					}
					output.WriteString(fmt.Sprintf("Env:          [%s]\n", strings.Join(envStrs, " ")))
				}
				if server.Transport != "" {
					output.WriteString(fmt.Sprintf("Transport:    %s\n", server.Transport))
				}
				if server.URL != "" {
					output.WriteString(fmt.Sprintf("URL:          %s\n", server.URL))
				}
				if server.Enabled != nil {
					output.WriteString(fmt.Sprintf("Enabled:      %t\n", *server.Enabled))
				}
				if len(server.Targets) > 0 {
					output.WriteString(fmt.Sprintf("Targets:      [%s]", strings.Join(server.Targets, " ")))
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
		}
		return nil, fmt.Errorf("MCP server not found: %s", name)
	default:
		return nil, fmt.Errorf("unsupported element type: %s", elementType)
	}
}

func HandleList(ctx context.Context, elementType string, cfg *config.Config) (*mcp.CallToolResult, error) {
	var items []string
	switch elementType {
	case "rules":
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
	case "sections":
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
	case "agents":
		if len(cfg.Agents) == 0 {
			items = append(items, "No agents found")
		} else {
			for _, agent := range cfg.Agents {
				desc := fmt.Sprintf("Name: %s", agent.Name)
				if agent.Description != "" {
					desc += fmt.Sprintf(", Description: %s", agent.Description)
				}
				if len(agent.Tools) > 0 {
					desc += fmt.Sprintf(", Tools: [%s]", strings.Join(agent.Tools, " "))
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
	case "outputs":
		if len(cfg.Outputs) == 0 {
			items = append(items, "No outputs found")
		} else {
			for _, output := range cfg.Outputs {
				desc := fmt.Sprintf("Path: %s", output.Path)
				if output.Type != "" {
					desc += fmt.Sprintf(", Type: %s", output.Type)
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
	case "commands":
		if len(cfg.Commands) == 0 {
			items = append(items, "No commands found")
		} else {
			for _, cmd := range cfg.Commands {
				desc := fmt.Sprintf("Name: %s", cmd.Name)
				if cmd.Description != "" {
					desc += fmt.Sprintf(", Description: %s", cmd.Description)
				}
				if len(cmd.Aliases) > 0 {
					desc += fmt.Sprintf(", Aliases: [%s]", strings.Join(cmd.Aliases, " "))
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
	case "mcp_servers":
		if len(cfg.MCPServers) == 0 {
			items = append(items, "No MCP servers found")
		} else {
			for _, server := range cfg.MCPServers {
				desc := fmt.Sprintf("Name: %s", server.Name)
				if server.Description != "" {
					desc += fmt.Sprintf(", Description: %s", server.Description)
				}
				if server.Command != "" {
					desc += fmt.Sprintf(", Command: %s", server.Command)
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
	case "includes":
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
	default:
		return nil, fmt.Errorf("unsupported element type: %s", elementType)
	}
}

func HandleUpdate(ctx context.Context, elementType, name string, updates map[string]interface{}, cfg *config.Config) (*mcp.CallToolResult, error) {
	switch elementType {
	case "metadata":
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
	case "extends":
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
	case "rules":
		for i, rule := range cfg.Rules {
			if rule.Name == name {
				for key, value := range updates {
					utils.LogIfErr(setFieldValue(&cfg.Rules[i], key, value))
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
	case "sections":
		for i, section := range cfg.Sections {
			if section.Name == name {
				for key, value := range updates {
					utils.LogIfErr(setFieldValue(&cfg.Sections[i], key, value))
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
	case "agents":
		for i, agent := range cfg.Agents {
			if agent.Name == name {
				for key, value := range updates {
					utils.LogIfErr(setFieldValue(&cfg.Agents[i], key, value))
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
	case "outputs":
		for i, output := range cfg.Outputs {
			if output.Path == name {
				for key, value := range updates {
					utils.LogIfErr(setFieldValue(&cfg.Outputs[i], key, value))
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
	case "commands":
		for i, cmd := range cfg.Commands {
			if cmd.Name == name {
				for key, value := range updates {
					utils.LogIfErr(setFieldValue(&cfg.Commands[i], key, value))
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
	case "mcp_servers":
		for i, server := range cfg.MCPServers {
			if server.Name == name {
				for key, value := range updates {
					utils.LogIfErr(setFieldValue(&cfg.MCPServers[i], key, value))
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
	default:
		return nil, fmt.Errorf("unsupported element type: %s", elementType)
	}
}

func HandleDelete(ctx context.Context, elementType, name string, cfg *config.Config) (*mcp.CallToolResult, error) {
	switch elementType {
	case "extends":
		cfg.Extends = ""
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: "Deleted extends configuration",
				},
			},
		}, nil
	case "rules":
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
	case "sections":
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
	case "agents":
		for i, agent := range cfg.Agents {
			if agent.Name == name {
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
	case "outputs":
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
	case "commands":
		for i, cmd := range cfg.Commands {
			if cmd.Name == name {
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
	case "mcp_servers":
		for i, server := range cfg.MCPServers {
			if server.Name == name {
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
	default:
		return nil, fmt.Errorf("unsupported element type: %s", elementType)
	}
}

func HandleAddToList(ctx context.Context, listType, value string, cfg *config.Config) (*mcp.CallToolResult, error) {
	switch listType {
	case "includes":
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
	case "includes":
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

// Helper functions for MCP handlers (used from internal/mcp/handlers package)
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

// HandleListJSON returns structured JSON data for MCP tools
func HandleListJSON(ctx context.Context, elementType string, cfg *config.Config) (*mcp.CallToolResult, error) {
	var responseData map[string]interface{}

	switch elementType {
	case "rules":
		responseData = map[string]interface{}{
			"rules": cfg.Rules,
		}
	case "sections":
		responseData = map[string]interface{}{
			"sections": cfg.Sections,
		}
	case "agents":
		responseData = map[string]interface{}{
			"agents": cfg.Agents,
		}
	case "outputs":
		responseData = map[string]interface{}{
			"outputs": cfg.Outputs,
		}
	case "mcp_servers":
		responseData = map[string]interface{}{
			"mcp_servers": cfg.MCPServers,
		}
	case "commands":
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

// HandleListJSONWithFilter returns structured JSON data for MCP tools with name filtering
func HandleListJSONWithFilter(ctx context.Context, elementType string, cfg *config.Config, nameFilter string) (*mcp.CallToolResult, error) {
	var responseData map[string]interface{}

	switch elementType {
	case "rules":
		var filteredRules []config.Rule
		for _, rule := range cfg.Rules {
			if nameFilter == "" || strings.Contains(strings.ToLower(rule.Name), strings.ToLower(nameFilter)) {
				filteredRules = append(filteredRules, rule)
			}
		}
		responseData = map[string]interface{}{
			"rules": filteredRules,
		}
	case "sections":
		var filteredSections []config.Section
		for _, section := range cfg.Sections {
			if nameFilter == "" || strings.Contains(strings.ToLower(section.Name), strings.ToLower(nameFilter)) {
				filteredSections = append(filteredSections, section)
			}
		}
		responseData = map[string]interface{}{
			"sections": filteredSections,
		}
	case "agents":
		var filteredAgents []config.Agent
		for _, agent := range cfg.Agents {
			if nameFilter == "" || strings.Contains(strings.ToLower(agent.Name), strings.ToLower(nameFilter)) {
				filteredAgents = append(filteredAgents, agent)
			}
		}
		responseData = map[string]interface{}{
			"agents": filteredAgents,
		}
	case "outputs":
		var filteredOutputs []config.Output
		for _, output := range cfg.Outputs {
			if nameFilter == "" || strings.Contains(strings.ToLower(output.Path), strings.ToLower(nameFilter)) {
				filteredOutputs = append(filteredOutputs, output)
			}
		}
		responseData = map[string]interface{}{
			"outputs": filteredOutputs,
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

// HandleGetJSON returns structured JSON data for a single item
func HandleGetJSON(ctx context.Context, elementType, name string, cfg *config.Config) (*mcp.CallToolResult, error) {
	var responseData map[string]interface{}

	switch elementType {
	case "rules":
		for _, rule := range cfg.Rules {
			if rule.Name == name {
				responseData = map[string]interface{}{
					"rule": rule,
				}
				break
			}
		}
	case "sections":
		for _, section := range cfg.Sections {
			if section.Name == name {
				responseData = map[string]interface{}{
					"section": section,
				}
				break
			}
		}
	case "agents":
		for _, agent := range cfg.Agents {
			if agent.Name == name {
				responseData = map[string]interface{}{
					"agent": agent,
				}
				break
			}
		}
	case "outputs":
		for _, output := range cfg.Outputs {
			if output.Path == name {
				responseData = map[string]interface{}{
					"output": output,
				}
				break
			}
		}
	case "mcp_servers":
		for _, server := range cfg.MCPServers {
			if server.Name == name {
				responseData = map[string]interface{}{
					"mcp_server": server,
				}
				break
			}
		}
	case "commands":
		for _, command := range cfg.Commands {
			if command.Name == name {
				responseData = map[string]interface{}{
					"command": command,
				}
				break
			}
		}
	default:
		return nil, fmt.Errorf("unknown element type: %s", elementType)
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

func HandleAddMCP(ctx context.Context, elementType string, element interface{}) (*mcp.CallToolResult, error) {
	cfg, configPath, err := loadConfigWithPath(ctx, "")
	if err != nil {
		return nil, err
	}

	// Add element to config
	var elementName string
	switch elementType {
	case "rules":
		rule := element.(*config.Rule) // Type assertion guaranteed by switch
		cfg.Rules = append(cfg.Rules, *rule)
		elementName = rule.Name
	case "sections":
		section := element.(*config.Section) // Type assertion guaranteed by switch
		cfg.Sections = append(cfg.Sections, *section)
		elementName = section.Name
	case "agents":
		agent := element.(*config.Agent) // Type assertion guaranteed by switch
		cfg.Agents = append(cfg.Agents, *agent)
		elementName = agent.Name
	case "outputs":
		output := element.(*config.Output) // Type assertion guaranteed by switch
		cfg.Outputs = append(cfg.Outputs, *output)
		elementName = output.Path
	case "mcp_servers":
		server := element.(*config.MCPServer) // Type assertion guaranteed by switch
		cfg.MCPServers = append(cfg.MCPServers, *server)
		elementName = server.Name
	case "commands":
		command := element.(*config.Command) // Type assertion guaranteed by switch
		cfg.Commands = append(cfg.Commands, *command)
		elementName = command.Name
	default:
		return nil, fmt.Errorf("unknown element type: %s", elementType)
	}

	if err := saveConfig(cfg, configPath); err != nil {
		return nil, err
	}

	// Return JSON success response
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

	// Update the element
	found := false
	switch elementType {
	case "rules":
		for i, rule := range cfg.Rules {
			if rule.Name == name {
				for key, value := range updates {
					utils.LogIfErr(setFieldValue(&cfg.Rules[i], key, value))
				}
				found = true
				break
			}
		}
	case "sections":
		for i, section := range cfg.Sections {
			if section.Name == name {
				for key, value := range updates {
					utils.LogIfErr(setFieldValue(&cfg.Sections[i], key, value))
				}
				found = true
				break
			}
		}
	case "agents":
		for i, agent := range cfg.Agents {
			if agent.Name == name {
				for key, value := range updates {
					utils.LogIfErr(setFieldValue(&cfg.Agents[i], key, value))
				}
				found = true
				break
			}
		}
	case "outputs":
		for i, output := range cfg.Outputs {
			if output.Path == name {
				for key, value := range updates {
					utils.LogIfErr(setFieldValue(&cfg.Outputs[i], key, value))
				}
				found = true
				break
			}
		}
	case "mcp_servers":
		for i, server := range cfg.MCPServers {
			if server.Name == name {
				for key, value := range updates {
					utils.LogIfErr(setFieldValue(&cfg.MCPServers[i], key, value))
				}
				found = true
				break
			}
		}
	case "commands":
		for i, command := range cfg.Commands {
			if command.Name == name {
				for key, value := range updates {
					utils.LogIfErr(setFieldValue(&cfg.Commands[i], key, value))
				}
				found = true
				break
			}
		}
	default:
		return nil, fmt.Errorf("unknown element type: %s", elementType)
	}

	if !found {
		return nil, fmt.Errorf("%s not found: %s", elementType, name)
	}

	if err := saveConfig(cfg, configPath); err != nil {
		return nil, err
	}

	// Return JSON success response
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

	// Delete the element
	found := false
	switch elementType {
	case "rules":
		for i, rule := range cfg.Rules {
			if rule.Name == name {
				cfg.Rules = append(cfg.Rules[:i], cfg.Rules[i+1:]...)
				found = true
				break
			}
		}
	case "sections":
		for i, section := range cfg.Sections {
			if section.Name == name {
				cfg.Sections = append(cfg.Sections[:i], cfg.Sections[i+1:]...)
				found = true
				break
			}
		}
	case "agents":
		for i, agent := range cfg.Agents {
			if agent.Name == name {
				cfg.Agents = append(cfg.Agents[:i], cfg.Agents[i+1:]...)
				found = true
				break
			}
		}
	case "outputs":
		for i, output := range cfg.Outputs {
			if output.Path == name {
				cfg.Outputs = append(cfg.Outputs[:i], cfg.Outputs[i+1:]...)
				found = true
				break
			}
		}
	case "mcp_servers":
		for i, server := range cfg.MCPServers {
			if server.Name == name {
				cfg.MCPServers = append(cfg.MCPServers[:i], cfg.MCPServers[i+1:]...)
				found = true
				break
			}
		}
	case "commands":
		for i, command := range cfg.Commands {
			if command.Name == name {
				cfg.Commands = append(cfg.Commands[:i], cfg.Commands[i+1:]...)
				found = true
				break
			}
		}
	default:
		return nil, fmt.Errorf("unknown element type: %s", elementType)
	}

	if !found {
		return nil, fmt.Errorf("%s not found: %s", elementType, name)
	}

	if err := saveConfig(cfg, configPath); err != nil {
		return nil, err
	}

	// Return JSON success response
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

// Helper utilities
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
		// For built-in templates, return the structured format with type and value
		return map[string]interface{}{
			"type":  "builtin",
			"value": templateValue,
		}
	case "file":
		// For file templates, return the structured format with type and value
		return map[string]interface{}{
			"type":  "file",
			"value": templateValue,
		}
	case "inline":
		// For inline templates, return the structured format with type and value
		return map[string]interface{}{
			"type":  "inline",
			"value": templateValue,
		}
	default:
		// Default to builtin template format
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
	// Create .continue directory if it doesn't exist
	continueDir := ".continue"
	if err := os.MkdirAll(continueDir, 0755); err != nil {
		return fmt.Errorf("failed to create .continue directory: %w", err)
	}

	// Create config.json for continue.dev
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
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		return fmt.Errorf("failed to write continue config: %w", err)
	}

	return nil
}

// Helper functions for config loading and saving
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

	// Create backup before writing
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

	// Write new configuration
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		// Try to restore backup on write failure
		if _, statErr := os.Stat(backupPath); statErr == nil {
			_ = os.Rename(backupPath, configPath)
		}
		return oops.Wrapf(err, "write configuration file")
	}

	// Remove backup on successful write
	_ = os.Remove(backupPath)

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
