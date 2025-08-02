package errors

import (
	"fmt"
	"strings"
)

// Helper functions for common error scenarios

// ConfigNotFound creates an error for missing configuration files
func ConfigNotFound(startDir string) *RichError {
	err := New(ErrorTypeConfigNotFound, "find config", 
		fmt.Errorf("no configuration file found"))
	err.Path = startDir
	err.Context["search_dir"] = startDir
	err.Context["supported_names"] = []string{
		"ai-rulez.yaml", "ai-rulez.yml",
		".ai-rulez.yaml", ".ai-rulez.yml",
		"ai_rulez.yaml", "ai_rulez.yml",
		".ai_rulez.yaml", ".ai_rulez.yml",
	}
	err.Suggestions = []string{
		"Run 'ai-rulez init' to create a new configuration file",
		"Create one of the supported config files: ai-rulez.yaml, .ai-rulez.yaml",
		"Check if you're in the correct directory",
		"Use --config flag to specify the config file path explicitly",
	}
	return err
}

// ConfigLoad creates an error for config loading failures
func ConfigLoad(filename string, underlying error) *RichError {
	err := New(ErrorTypeConfig, "load config", underlying)
	err.Path = filename
	err.Context["filename"] = filename
	return err
}

// ConfigParse creates an error for YAML parsing failures
func ConfigParse(filename string, underlying error) *RichError {
	err := New(ErrorTypeConfigInvalid, "parse config", underlying).
		WithPath(filename).
		WithContext("filename", filename).
		WithSuggestion("Check the YAML syntax - ensure proper indentation").
		WithSuggestion("Validate your YAML at: https://www.yamllint.com/").
		WithSuggestion("Common issues: tabs instead of spaces, missing colons, incorrect indentation")
	
	// Try to extract line information from YAML errors
	if strings.Contains(underlying.Error(), "line") {
		err.WithContext("parse_error", underlying.Error()) //nolint:errcheck
	}
	
	return err
}

// SchemaValidation creates an error for schema validation failures
func SchemaValidation(filename string, validationErrors []string) *RichError {
	err := New(ErrorTypeConfigSchema, "validate schema",
		fmt.Errorf("configuration validation failed: %d errors", len(validationErrors)))
	err.Path = filename
	err.Context["filename"] = filename
	err.Context["errors"] = validationErrors
	err.Context["error_count"] = len(validationErrors)
	
	err.Suggestions = []string{
		"Check the YAML syntax using a YAML validator",
		"Ensure all required fields are present (metadata.name, outputs)",
		"Verify the structure matches the schema at: https://github.com/Goldziher/ai-rulez/blob/main/schema/ai-rules-v1.schema.json",
		"Run 'ai-rulez validate' for detailed validation output",
	}
	
	// Add specific suggestions based on common validation errors
	for _, vErr := range validationErrors {
		if strings.Contains(vErr, "metadata") {
			err.WithSuggestion("Ensure 'metadata' section exists with 'name' field") //nolint:errcheck
		}
		if strings.Contains(vErr, "outputs") {
			err.WithSuggestion("Add at least one output file in the 'outputs' section") //nolint:errcheck
		}
		if strings.Contains(vErr, "priority") {
			err.WithSuggestion("Priority must be a number between 1 and 20") //nolint:errcheck
		}
	}
	
	return err
}

// CircularInclude creates an error for circular include detection
func CircularInclude(includeChain []string) *RichError {
	err := New(ErrorTypeConfigCircular, "load includes",
		fmt.Errorf("circular include detected"))
	
	if len(includeChain) > 0 {
		err.Path = includeChain[len(includeChain)-1]
	}
	
	err.Context["include_chain"] = includeChain
	err.Context["circular_file"] = includeChain[len(includeChain)-1]
	
	err.Suggestions = []string{
		fmt.Sprintf("Remove the circular reference in %s", includeChain[len(includeChain)-1]),
		"Restructure your includes to avoid circular dependencies",
		"Use a different include strategy (e.g., separate common rules)",
		"Consider merging the circular files into a single file",
	}
	
	return err
}

// TemplateNotFound creates an error for missing templates
func TemplateNotFound(name string, available []string) *RichError {
	err := New(ErrorTypeTemplateNotFound, "load template",
		fmt.Errorf("template '%s' not found", name))
	err.Context["requested"] = name
	err.Context["available"] = available
	err.Suggestions = []string{
		fmt.Sprintf("Use one of the available templates: %s", strings.Join(available, ", ")),
		"Use 'default' for the standard template",
		"Define a custom template inline using Go template syntax",
		"Reference an external template file using '@filename'",
	}
	return err
}

// TemplateParse creates an error for template parsing failures
func TemplateParse(name string, underlying error) *RichError {
	err := New(ErrorTypeTemplateSyntax, "parse template", underlying).
		WithContext("template", name).
		WithSuggestion("Check the template syntax - common issues: unclosed '{{', invalid dot notation").
		WithSuggestion("Validate your template at: https://golang.org/pkg/text/template/").
		WithSuggestion("Use a built-in template as a reference: 'default' or 'documentation'")
	
	// Extract line information if available
	if strings.Contains(underlying.Error(), "line") {
		err.WithContext("parse_error", underlying.Error()) //nolint:errcheck
	}
	
	return err
}

// TemplateExecute creates an error for template execution failures
func TemplateExecute(name string, underlying error) *RichError {
	err := New(ErrorTypeTemplate, "execute template", underlying).
		WithContext("template", name).
		WithSuggestion("Check that all template variables are available in the data").
		WithSuggestion("Common issue: accessing a field that doesn't exist").
		WithSuggestion("Use {{if}} checks for optional fields")
	
	return err
}

// FileRead creates an error for file read failures
func FileRead(path string, underlying error) *RichError {
	err := New(ErrorTypeFileRead, "read file", underlying).
		WithPath(path).
		WithSuggestion("Check if the file exists: %s", path).
		WithSuggestion("Verify you have read permissions for the file").
		WithSuggestion("Ensure the path is correct and accessible")
	
	return err
}

// FileWrite creates an error for file write failures
func FileWrite(path string, underlying error) *RichError {
	err := New(ErrorTypeFileWrite, "write file", underlying).
		WithPath(path).
		WithSuggestion("Check if you have write permissions for: %s", path).
		WithSuggestion("Ensure the parent directory exists").
		WithSuggestion("Check available disk space")
	
	return err
}

// FilePermission creates an error for permission issues
func FilePermission(path string, operation string) *RichError {
	err := New(ErrorTypeFilePermission, operation,
		fmt.Errorf("permission denied")).
		WithPath(path).
		WithSuggestion("Check file permissions: try 'ls -la %s'", path).
		WithSuggestion("You may need to run with appropriate permissions").
		WithSuggestion("Ensure the file/directory is not locked by another process")
	
	return err
}

// ValidationRequired creates an error for missing required fields
func ValidationRequired(field string, context string) *RichError {
	err := New(ErrorTypeValidationRequired, "validate field",
		fmt.Errorf("required field '%s' is missing", field)).
		WithContext("field", field).
		WithContext("context", context).
		WithSuggestion("Add the required field '%s' to your configuration", field).
		WithSuggestion("Check the documentation for the correct field structure")
	
	return err
}

// CommandArgument creates an error for invalid command arguments
func CommandArgument(arg string, value string, reason string) *RichError {
	err := New(ErrorTypeCommandArgument, "parse argument",
		fmt.Errorf("invalid argument '%s': %s", arg, reason)).
		WithContext("argument", arg).
		WithContext("value", value).
		WithContext("reason", reason).
		WithSuggestion("Check the command help: 'ai-rulez [command] --help'").
		WithSuggestion("Ensure the argument value is in the correct format")
	
	return err
}

// MCPError creates an error for MCP protocol issues
func MCPError(operation string, underlying error) *RichError {
	err := New(ErrorTypeMCP, operation, underlying).
		WithSuggestion("Check the MCP connection and try again").
		WithSuggestion("Ensure the MCP server is running correctly").
		WithSuggestion("Verify the request format matches the MCP protocol")
	
	return err
}