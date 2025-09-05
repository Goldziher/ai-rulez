package crud

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/spf13/cobra"
)

const (
	fieldNameName     = "Name"
	fieldTypePriority = "priority"
)

type EntityDescriptor struct {
	Singular    string
	Plural      string
	ElementType string
	Fields      []FieldDescriptor
	NewEntity   func() interface{}
}

type FieldDescriptor struct {
	Name        string
	FlagName    string
	ShortFlag   string
	Description string
	Type        string
	Required    bool
	Default     interface{}
}

func CreateCRUDCommands(desc *EntityDescriptor) *CRUDCommands {
	return &CRUDCommands{
		Add:    createAddCommand(desc),
		Update: createUpdateCommand(desc),
		Delete: createDeleteCommand(desc),
		Get:    createGetCommand(desc),
		List:   createListCommand(desc),
	}
}

type CRUDCommands struct {
	Add    *cobra.Command
	Update *cobra.Command
	Delete *cobra.Command
	Get    *cobra.Command
	List   *cobra.Command
}

func createAddCommand(desc *EntityDescriptor) *cobra.Command {
	flagValues := make(map[string]interface{})

	cmd := &cobra.Command{
		Use:   fmt.Sprintf("%s [name]", desc.Singular),
		Short: fmt.Sprintf("Add a new %s to the configuration", desc.Singular),
		Long:  fmt.Sprintf("Adds a new %s to the %s list in your ai_rulez.yaml file.", desc.Singular, desc.Plural),
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			name := args[0]
			entity := desc.NewEntity()

			if desc.Singular == "output" {
				setFieldValue(entity, "Path", name)
			} else {
				setFieldValue(entity, "Name", name)
			}

			for _, field := range desc.Fields {
				if field.Name == fieldNameName {
					continue
				}

				if cmd.Flags().Changed(field.FlagName) {
					value := flagValues[field.FlagName]
					if field.Type == fieldTypePriority {
						if strVal, ok := value.(*string); ok && strVal != nil {
							priority, err := config.ParsePriority(*strVal)
							if err != nil {
								crud.FmtError("%v", err)
								return
							}
							setFieldValue(entity, field.Name, priority)
						}
					} else {
						setFieldValue(entity, field.Name, value)
					}
				}
			}

			crud.AddElement(desc.ElementType, entity)
		},
	}

	for _, field := range desc.Fields {
		if field.Name == fieldNameName {
			continue
		}

		switch field.Type {
		case "string":
			var strVal string
			flagValues[field.FlagName] = &strVal
			defaultVal := ""
			if field.Default != nil {
				defaultVal = field.Default.(string)
			}
			if field.ShortFlag != "" {
				cmd.Flags().StringVarP(&strVal, field.FlagName, field.ShortFlag, defaultVal, field.Description)
			} else {
				cmd.Flags().StringVar(&strVal, field.FlagName, defaultVal, field.Description)
			}
			if field.Required {
				if err := cmd.MarkFlagRequired(field.FlagName); err != nil {
					panic(fmt.Sprintf("failed to mark flag as required: %v", err))
				}
			}

		case "[]string":
			var sliceVal []string
			flagValues[field.FlagName] = &sliceVal
			defaultVal := []string{}
			if field.Default != nil {
				defaultVal = field.Default.([]string)
			}
			if field.ShortFlag != "" {
				cmd.Flags().StringSliceVarP(&sliceVal, field.FlagName, field.ShortFlag, defaultVal, field.Description)
			} else {
				cmd.Flags().StringSliceVar(&sliceVal, field.FlagName, defaultVal, field.Description)
			}

		case "bool":
			var boolVal bool
			flagValues[field.FlagName] = &boolVal
			defaultVal := false
			if field.Default != nil {
				defaultVal = field.Default.(bool)
			}
			if field.ShortFlag != "" {
				cmd.Flags().BoolVarP(&boolVal, field.FlagName, field.ShortFlag, defaultVal, field.Description)
			} else {
				cmd.Flags().BoolVar(&boolVal, field.FlagName, defaultVal, field.Description)
			}

		case fieldTypePriority:
			var strVal string
			flagValues[field.FlagName] = &strVal
			defaultVal := "medium"
			if field.Default != nil {
				defaultVal = field.Default.(string)
			}
			if field.ShortFlag != "" {
				cmd.Flags().StringVarP(&strVal, field.FlagName, field.ShortFlag, defaultVal, field.Description)
			} else {
				cmd.Flags().StringVar(&strVal, field.FlagName, defaultVal, field.Description)
			}
		}
	}

	return cmd
}

func createUpdateCommand(desc *EntityDescriptor) *cobra.Command {
	flagValues := make(map[string]interface{})

	cmd := &cobra.Command{
		Use:   fmt.Sprintf("%s [name]", desc.Singular),
		Short: fmt.Sprintf("Update an existing %s", desc.Singular),
		Long:  fmt.Sprintf("Updates an existing %s in your ai_rulez.yaml file.", desc.Singular),
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			name := args[0]
			updates := make(map[string]interface{})

			for _, field := range desc.Fields {
				if field.Name == fieldNameName {
					continue
				}

				if cmd.Flags().Changed(field.FlagName) {
					fieldName := capitalizeFirst(field.Name)
					value := flagValues[field.FlagName]

					if field.Type == fieldTypePriority {
						if strVal, ok := value.(*string); ok && strVal != nil {
							priority, err := config.ParsePriority(*strVal)
							if err != nil {
								crud.FmtError("%v", err)
								return
							}
							updates[fieldName] = priority
						}
					} else {
						updates[fieldName] = dereferenceValue(value)
					}
				}
			}

			crud.UpdateElement(desc.ElementType, name, updates)
		},
	}

	for _, field := range desc.Fields {
		if field.Name == fieldNameName {
			continue
		}

		switch field.Type {
		case "string":
			var strVal string
			flagValues[field.FlagName] = &strVal
			if field.ShortFlag != "" {
				cmd.Flags().StringVarP(&strVal, field.FlagName, field.ShortFlag, "", fmt.Sprintf("New %s", field.Description))
			} else {
				cmd.Flags().StringVar(&strVal, field.FlagName, "", fmt.Sprintf("New %s", field.Description))
			}

		case "[]string":
			var sliceVal []string
			flagValues[field.FlagName] = &sliceVal
			if field.ShortFlag != "" {
				cmd.Flags().StringSliceVarP(&sliceVal, field.FlagName, field.ShortFlag, []string{}, fmt.Sprintf("New %s", field.Description))
			} else {
				cmd.Flags().StringSliceVar(&sliceVal, field.FlagName, []string{}, fmt.Sprintf("New %s", field.Description))
			}

		case "bool":
			var boolVal bool
			flagValues[field.FlagName] = &boolVal
			if field.ShortFlag != "" {
				cmd.Flags().BoolVarP(&boolVal, field.FlagName, field.ShortFlag, false, fmt.Sprintf("New %s", field.Description))
			} else {
				cmd.Flags().BoolVar(&boolVal, field.FlagName, false, fmt.Sprintf("New %s", field.Description))
			}

		case fieldTypePriority:
			var strVal string
			flagValues[field.FlagName] = &strVal
			if field.ShortFlag != "" {
				cmd.Flags().StringVarP(&strVal, field.FlagName, field.ShortFlag, "", fmt.Sprintf("New %s", field.Description))
			} else {
				cmd.Flags().StringVar(&strVal, field.FlagName, "", fmt.Sprintf("New %s", field.Description))
			}
		}
	}

	return cmd
}

func createDeleteCommand(desc *EntityDescriptor) *cobra.Command {
	return &cobra.Command{
		Use:   fmt.Sprintf("%s [name]", desc.Singular),
		Short: fmt.Sprintf("Delete a %s from the configuration", desc.Singular),
		Long:  fmt.Sprintf("Deletes a %s by name from your ai_rulez.yaml file.", desc.Singular),
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			name := args[0]
			crud.DeleteElement(desc.ElementType, name)
		},
	}
}

func createGetCommand(desc *EntityDescriptor) *cobra.Command {
	return &cobra.Command{
		Use:   fmt.Sprintf("%s [name]", desc.Singular),
		Short: fmt.Sprintf("Get a %s from the configuration", desc.Singular),
		Long:  fmt.Sprintf("Retrieves a %s by name from your ai_rulez.yaml file.", desc.Singular),
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			name := args[0]
			crud.GetElement(desc.ElementType, name)
		},
	}
}

func createListCommand(desc *EntityDescriptor) *cobra.Command {
	return &cobra.Command{
		Use:     desc.Plural,
		Short:   fmt.Sprintf("List all %s in the configuration", desc.Plural),
		Long:    fmt.Sprintf("Lists all %s defined in your AI rules configuration.", desc.Plural),
		Aliases: []string{desc.Singular},
		Run: func(cmd *cobra.Command, args []string) {
			crud.ListElements(desc.ElementType)
		},
	}
}

func setFieldValue(entity interface{}, fieldName string, value interface{}) {
	v := reflect.ValueOf(entity).Elem()
	field := v.FieldByName(fieldName)
	if field.IsValid() && field.CanSet() {
		if ptr, ok := value.(*string); ok && ptr != nil {
			field.SetString(*ptr)
			return
		}
		if ptr, ok := value.(*[]string); ok && ptr != nil {
			field.Set(reflect.ValueOf(*ptr))
			return
		}
		if ptr, ok := value.(*bool); ok && ptr != nil {
			field.SetBool(*ptr)
			return
		}
		if value != nil {
			field.Set(reflect.ValueOf(value))
		}
	}
}

func dereferenceValue(value interface{}) interface{} {
	v := reflect.ValueOf(value)
	if v.Kind() == reflect.Ptr && !v.IsNil() {
		return v.Elem().Interface()
	}
	return value
}

func capitalizeFirst(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

var RuleDescriptor = EntityDescriptor{
	Singular:    "rule",
	Plural:      "rules",
	ElementType: "rules",
	Fields: []FieldDescriptor{
		{Name: "ID", FlagName: "id", Description: "Optional unique identifier for the rule", Type: "string"},
		{Name: "Name", FlagName: "name", Description: "Name of the rule", Type: "string"},
		{Name: "Content", FlagName: "content", ShortFlag: "c", Description: "Content of the rule", Type: "string", Required: true},
		{Name: "Priority", FlagName: "priority", ShortFlag: "p", Description: "Priority of the rule (critical, high, medium, low, minimal)", Type: "priority", Default: "medium"},
		{Name: "Targets", FlagName: "target", ShortFlag: "t", Description: "Output target for this rule (can be specified multiple times)", Type: "[]string"},
	},
	NewEntity: func() interface{} { return &config.Rule{} },
}

var SectionDescriptor = EntityDescriptor{
	Singular:    "section",
	Plural:      "sections",
	ElementType: "sections",
	Fields: []FieldDescriptor{
		{Name: "ID", FlagName: "id", Description: "Optional unique identifier for the section", Type: "string"},
		{Name: "Name", FlagName: "name", Description: "Name of the section", Type: "string"},
		{Name: "Content", FlagName: "content", ShortFlag: "c", Description: "Content of the section", Type: "string", Required: true},
		{Name: "Priority", FlagName: "priority", ShortFlag: "p", Description: "Priority of the section (critical, high, medium, low, minimal)", Type: "priority", Default: "medium"},
		{Name: "Targets", FlagName: "target", ShortFlag: "t", Description: "Output target for this section (can be specified multiple times)", Type: "[]string"},
	},
	NewEntity: func() interface{} { return &config.Section{} },
}

var AgentDescriptor = EntityDescriptor{
	Singular:    "agent",
	Plural:      "agents",
	ElementType: "agents",
	Fields: []FieldDescriptor{
		{Name: "ID", FlagName: "id", Description: "Optional unique identifier for the agent", Type: "string"},
		{Name: "Name", FlagName: "name", Description: "Name of the agent", Type: "string"},
		{Name: "Description", FlagName: "description", ShortFlag: "d", Description: "Description of the agent's purpose", Type: "string", Required: true},
		{Name: "Priority", FlagName: "priority", ShortFlag: "p", Description: "Priority of the agent (critical, high, medium, low, minimal)", Type: "priority", Default: "medium"},
		{Name: "Tools", FlagName: "tools", Description: "Tools available to the agent", Type: "[]string"},
		{Name: "Model", FlagName: "model", ShortFlag: "m", Description: "Model to use for the agent", Type: "string"},
		{Name: "SystemPrompt", FlagName: "system-prompt", ShortFlag: "s", Description: "System prompt for the agent", Type: "string"},
		{Name: "Targets", FlagName: "target", ShortFlag: "t", Description: "Output target for this agent (can be specified multiple times)", Type: "[]string"},
	},
	NewEntity: func() interface{} { return &config.Agent{} },
}

var OutputDescriptor = EntityDescriptor{
	Singular:    "output",
	Plural:      "outputs",
	ElementType: "outputs",
	Fields: []FieldDescriptor{
		{Name: "Path", FlagName: "path", Description: "Output path", Type: "string"},
		{Name: "Type", FlagName: "type", ShortFlag: "t", Description: "Type of output (rule, agent, all)", Type: "string"},
		{Name: "NamingScheme", FlagName: "naming-scheme", Description: "File naming scheme", Type: "string"},
	},
	NewEntity: func() interface{} { return &config.Output{} },
}

var CommandDescriptor = EntityDescriptor{
	Singular:    "command",
	Plural:      "commands",
	ElementType: "commands",
	Fields: []FieldDescriptor{
		{Name: "ID", FlagName: "id", Description: "Optional unique identifier for the command", Type: "string"},
		{Name: "Name", FlagName: "name", Description: "Name of the command", Type: "string"},
		{Name: "Description", FlagName: "description", ShortFlag: "d", Description: "Description of the command", Type: "string"},
		{Name: "Command", FlagName: "command", ShortFlag: "c", Description: "Command to execute", Type: "string", Required: true},
		{Name: "Args", FlagName: "args", Description: "Arguments for the command", Type: "[]string"},
		{Name: "Env", FlagName: "env", Description: "Environment variables", Type: "[]string"},
		{Name: "Targets", FlagName: "target", ShortFlag: "t", Description: "Output target for this command (can be specified multiple times)", Type: "[]string"},
	},
	NewEntity: func() interface{} { return &config.Command{} },
}

var MCPServerDescriptor = EntityDescriptor{
	Singular:    "mcp-server",
	Plural:      "mcp-servers",
	ElementType: "mcp_servers",
	Fields: []FieldDescriptor{
		{Name: "ID", FlagName: "id", Description: "Optional unique identifier for the MCP server", Type: "string"},
		{Name: "Name", FlagName: "name", Description: "Name of the MCP server", Type: "string"},
		{Name: "Description", FlagName: "description", ShortFlag: "d", Description: "Description of the MCP server", Type: "string"},
		{Name: "Command", FlagName: "command", ShortFlag: "c", Description: "Command to start the server", Type: "string"},
		{Name: "Args", FlagName: "args", Description: "Arguments for the server", Type: "[]string"},
		{Name: "Transport", FlagName: "transport", Description: "Transport type (stdio, http)", Type: "string", Default: "stdio"},
		{Name: "URL", FlagName: "url", ShortFlag: "u", Description: "URL for HTTP transport", Type: "string"},
		{Name: "Enabled", FlagName: "enabled", ShortFlag: "e", Description: "Whether the server is enabled", Type: "bool", Default: true},
		{Name: "Targets", FlagName: "target", ShortFlag: "t", Description: "Output target for this server (can be specified multiple times)", Type: "[]string"},
	},
	NewEntity: func() interface{} { return &config.MCPServer{} },
}
