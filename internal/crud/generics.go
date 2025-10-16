package crud

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type EntityOps[T any] struct {
	GetSlice     func(*config.Config) []T
	SetSlice     func(*config.Config, []T)
	ElementType  string
	GetName      func(T) string
	FormatForCLI func(T) string
}

var (
	RuleOps = EntityOps[config.Rule]{
		GetSlice:    func(cfg *config.Config) []config.Rule { return cfg.Rules },
		SetSlice:    func(cfg *config.Config, items []config.Rule) { cfg.Rules = items },
		ElementType: ElementTypeRules,
		GetName:     func(r config.Rule) string { return r.Name },
		FormatForCLI: func(r config.Rule) string {
			targetsStr := "[]"
			if len(r.Targets) > 0 {
				targetsStr = "[" + strings.Join(r.Targets, " ") + "]"
			}
			result := fmt.Sprintf("Name:     %s\nPriority: %s\nContent:  %s\n", r.Name, r.Priority, r.Content)
			if r.ID != "" {
				result = fmt.Sprintf("Name:     %s\nID:       %s\nPriority: %s\nContent:  %s\n", r.Name, r.ID, r.Priority, r.Content)
			}
			if len(r.Targets) > 0 {
				result += fmt.Sprintf("Targets:  %s\n", targetsStr)
			}
			return result
		},
	}

	SectionOps = EntityOps[config.Section]{
		GetSlice:    func(cfg *config.Config) []config.Section { return cfg.Sections },
		SetSlice:    func(cfg *config.Config, items []config.Section) { cfg.Sections = items },
		ElementType: ElementTypeSections,
		GetName:     func(s config.Section) string { return s.Name },
		FormatForCLI: func(s config.Section) string {
			targetsStr := "[]"
			if len(s.Targets) > 0 {
				targetsStr = "[" + strings.Join(s.Targets, " ") + "]"
			}
			result := fmt.Sprintf("Name:     %s\nID:       %s\nPriority: %s\nContent:  %s\n", s.Name, s.ID, s.Priority, s.Content)
			if len(s.Targets) > 0 {
				result += fmt.Sprintf("Targets:  %s\n", targetsStr)
			}
			return result
		},
	}

	AgentOps = EntityOps[config.Agent]{
		GetSlice:    func(cfg *config.Config) []config.Agent { return cfg.Agents },
		SetSlice:    func(cfg *config.Config, items []config.Agent) { cfg.Agents = items },
		ElementType: ElementTypeAgents,
		GetName:     func(a config.Agent) string { return a.Name },
		FormatForCLI: func(a config.Agent) string {
			toolsStr := "[]"
			if len(a.Tools) > 0 {
				toolsStr = "[" + strings.Join(a.Tools, " ") + "]"
			}
			return fmt.Sprintf("Name:         %s\nID:           %s\nDescription:   %s\nPriority:     %s\nSystem Prompt: %s\nTools:        %s\n",
				a.Name, a.ID, a.Description, a.Priority, a.SystemPrompt, toolsStr)
		},
	}

	OutputOps = EntityOps[config.Output]{
		GetSlice:    func(cfg *config.Config) []config.Output { return cfg.Outputs },
		SetSlice:    func(cfg *config.Config, items []config.Output) { cfg.Outputs = items },
		ElementType: ElementTypeOutputs,
		GetName:     func(o config.Output) string { return o.Path },
		FormatForCLI: func(o config.Output) string {
			result := fmt.Sprintf("Path:         %s\nType:         %s\n", o.Path, o.Type)
			if o.NamingScheme != "" {
				result += fmt.Sprintf("Naming Scheme: %s\n", o.NamingScheme)
			}
			return result
		},
	}

	CommandOps = EntityOps[config.Command]{
		GetSlice:    func(cfg *config.Config) []config.Command { return cfg.Commands },
		SetSlice:    func(cfg *config.Config, items []config.Command) { cfg.Commands = items },
		ElementType: ElementTypeCommands,
		GetName:     func(c config.Command) string { return c.Name },
		FormatForCLI: func(c config.Command) string {
			return fmt.Sprintf("Name:         %s\nDescription:  %s\n",
				c.Name, c.Description)
		},
	}

	MCPServerOps = EntityOps[config.MCPServer]{
		GetSlice:    func(cfg *config.Config) []config.MCPServer { return cfg.MCPServers },
		SetSlice:    func(cfg *config.Config, items []config.MCPServer) { cfg.MCPServers = items },
		ElementType: ElementTypeMCPServers,
		GetName:     func(m config.MCPServer) string { return m.Name },
		FormatForCLI: func(m config.MCPServer) string {
			argsStr := "[]"
			if len(m.Args) > 0 {
				argsStr = "[" + strings.Join(m.Args, " ") + "]"
			}
			result := fmt.Sprintf("Name:         %s\nDescription:  %s\n", m.Name, m.Description)
			if m.Command != "" {
				result += fmt.Sprintf("Command:      %s\n", m.Command)
			}
			if len(m.Args) > 0 {
				result += fmt.Sprintf("Args:         %s\n", argsStr)
			}
			if m.Transport != "" {
				result += fmt.Sprintf("Transport:    %s\n", m.Transport)
			}
			if m.URL != "" {
				result += fmt.Sprintf("URL:          %s\n", m.URL)
			}
			return result
		},
	}
)

func genericAdd[T any](ops EntityOps[T], element T, cfg *config.Config) (*sdkmcp.CallToolResult, error) {
	items := ops.GetSlice(cfg)
	items = append(items, element)
	ops.SetSlice(cfg, items)

	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{
			&sdkmcp.TextContent{
				Text: fmt.Sprintf("Added %s: %s", getSingular(ops.ElementType), ops.GetName(element)),
			},
		},
	}, nil
}

func genericGet[T any](ops EntityOps[T], name string, cfg *config.Config) (*sdkmcp.CallToolResult, error) {
	items := ops.GetSlice(cfg)

	for _, item := range items {
		if ops.GetName(item) == name {
			formattedData := ops.FormatForCLI(item)
			return &sdkmcp.CallToolResult{
				Content: []sdkmcp.Content{
					&sdkmcp.TextContent{Text: formattedData},
				},
			}, nil
		}
	}

	return nil, fmt.Errorf("%s '%s' not found", getSingular(ops.ElementType), name)
}

func genericList[T any](ops EntityOps[T], cfg *config.Config) (*sdkmcp.CallToolResult, error) {
	items := ops.GetSlice(cfg)

	if len(items) == 0 {
		return &sdkmcp.CallToolResult{
			Content: []sdkmcp.Content{
				&sdkmcp.TextContent{Text: fmt.Sprintf("No %s found", ops.ElementType)},
			},
		}, nil
	}

	var names []string
	for _, item := range items {
		names = append(names, ops.GetName(item))
	}

	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{
			&sdkmcp.TextContent{Text: strings.Join(names, "\n")},
		},
	}, nil
}

func genericUpdate[T any](ops EntityOps[T], name string, updates map[string]interface{}, cfg *config.Config) (*sdkmcp.CallToolResult, error) {
	items := ops.GetSlice(cfg)

	index := slices.IndexFunc(items, func(item T) bool {
		return ops.GetName(item) == name
	})

	if index == -1 {
		return nil, fmt.Errorf("%s '%s' not found", getSingular(ops.ElementType), name)
	}

	jsonData, err := json.Marshal(items[index])
	if err != nil {
		return nil, fmt.Errorf("error marshaling %s: %w", getSingular(ops.ElementType), err)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return nil, fmt.Errorf("error unmarshaling %s: %w", getSingular(ops.ElementType), err)
	}

	for key, value := range updates {
		data[key] = value
	}

	updatedJSON, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("error marshaling updated %s: %w", getSingular(ops.ElementType), err)
	}

	var updatedItem T
	if err := json.Unmarshal(updatedJSON, &updatedItem); err != nil {
		return nil, fmt.Errorf("error unmarshaling updated %s: %w", getSingular(ops.ElementType), err)
	}

	items[index] = updatedItem
	ops.SetSlice(cfg, items)

	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{
			&sdkmcp.TextContent{Text: fmt.Sprintf("Updated %s: %s", getSingular(ops.ElementType), name)},
		},
	}, nil
}

func genericDelete[T any](ops EntityOps[T], name string, cfg *config.Config) (*sdkmcp.CallToolResult, error) {
	items := ops.GetSlice(cfg)

	newItems := slices.DeleteFunc(items, func(item T) bool {
		return ops.GetName(item) == name
	})

	if len(newItems) == len(items) {
		return nil, fmt.Errorf("%s '%s' not found", getSingular(ops.ElementType), name)
	}

	ops.SetSlice(cfg, newItems)

	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{
			&sdkmcp.TextContent{Text: fmt.Sprintf("Deleted %s: %s", getSingular(ops.ElementType), name)},
		},
	}, nil
}

func genericListJSON[T any](ops EntityOps[T], cfg *config.Config) (*sdkmcp.CallToolResult, error) {
	items := ops.GetSlice(cfg)

	jsonData, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("error marshaling %s: %w", ops.ElementType, err)
	}

	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{
			&sdkmcp.TextContent{Text: string(jsonData)},
		},
	}, nil
}

func genericListJSONWithFilter[T any](ops EntityOps[T], cfg *config.Config, nameFilter string) (*sdkmcp.CallToolResult, error) {
	items := ops.GetSlice(cfg)

	var filtered []T
	for _, item := range items {
		if strings.Contains(strings.ToLower(ops.GetName(item)), strings.ToLower(nameFilter)) {
			filtered = append(filtered, item)
		}
	}

	jsonData, err := json.MarshalIndent(filtered, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("error marshaling %s: %w", ops.ElementType, err)
	}

	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{
			&sdkmcp.TextContent{Text: string(jsonData)},
		},
	}, nil
}

func genericGetJSON[T any](ops EntityOps[T], name string, cfg *config.Config) (*sdkmcp.CallToolResult, error) {
	items := ops.GetSlice(cfg)

	for _, item := range items {
		if ops.GetName(item) == name {
			jsonData, err := json.MarshalIndent(item, "", "  ")
			if err != nil {
				return nil, fmt.Errorf("error marshaling %s: %w", getSingular(ops.ElementType), err)
			}
			return &sdkmcp.CallToolResult{
				Content: []sdkmcp.Content{
					&sdkmcp.TextContent{Text: string(jsonData)},
				},
			}, nil
		}
	}

	return nil, fmt.Errorf("%s '%s' not found", getSingular(ops.ElementType), name)
}
