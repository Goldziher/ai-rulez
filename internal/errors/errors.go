// Package errors provides rich error handling for ai-rulez
package errors

import (
	"fmt"
	"runtime"
	"strings"
)

// ErrorType represents the category of error
type ErrorType int

const (
	// Configuration errors
	ErrorTypeConfig ErrorType = iota
	ErrorTypeConfigNotFound
	ErrorTypeConfigInvalid
	ErrorTypeConfigSchema
	ErrorTypeConfigCircular

	// File operation errors
	ErrorTypeFileRead
	ErrorTypeFileWrite
	ErrorTypeFilePermission

	// Template errors
	ErrorTypeTemplate
	ErrorTypeTemplateNotFound
	ErrorTypeTemplateSyntax

	// Validation errors
	ErrorTypeValidation
	ErrorTypeValidationRequired
	ErrorTypeValidationFormat

	// Command errors
	ErrorTypeCommand
	ErrorTypeCommandArgument

	// Generation errors
	ErrorTypeGeneration
	ErrorTypeGenerationOutputNotFound

	// MCP protocol errors
	ErrorTypeMCP
)

// String returns the string representation of the error type
func (t ErrorType) String() string {
	// Use a map for simpler lookup to reduce complexity
	errorTypeStrings := map[ErrorType]string{
		ErrorTypeConfig:                   "config",
		ErrorTypeConfigNotFound:           "config_not_found",
		ErrorTypeConfigInvalid:            "config_invalid",
		ErrorTypeConfigSchema:             "config_schema",
		ErrorTypeConfigCircular:           "config_circular",
		ErrorTypeFileRead:                 "file_read",
		ErrorTypeFileWrite:                "file_write",
		ErrorTypeFilePermission:           "file_permission",
		ErrorTypeTemplate:                 "template",
		ErrorTypeTemplateNotFound:         "template_not_found",
		ErrorTypeTemplateSyntax:           "template_syntax",
		ErrorTypeValidation:               "validation",
		ErrorTypeValidationRequired:       "validation_required",
		ErrorTypeValidationFormat:         "validation_format",
		ErrorTypeCommand:                  "command",
		ErrorTypeCommandArgument:          "command_argument",
		ErrorTypeGeneration:               "generation",
		ErrorTypeGenerationOutputNotFound: "generation_output_not_found",
		ErrorTypeMCP:                      "mcp",
	}

	if str, ok := errorTypeStrings[t]; ok {
		return str
	}
	return "unknown"
}

// RichError provides detailed error information with context and suggestions
type RichError struct {
	Type        ErrorType
	Op          string                 // Operation that failed
	Path        string                 // File path if applicable
	Err         error                  // Underlying error
	Context     map[string]interface{} // Additional context
	Suggestions []string               // Actionable suggestions for recovery
	File        string                 // Source file where error occurred
	Line        int                    // Source line where error occurred
}

// Error implements the error interface
func (e *RichError) Error() string {
	var b strings.Builder

	// Primary error message
	if e.Op != "" {
		b.WriteString(e.Op)
		b.WriteString(": ")
	}

	if e.Err != nil {
		b.WriteString(e.Err.Error())
	}

	// Add path context if available
	if e.Path != "" {
		b.WriteString(fmt.Sprintf(" (path: %s)", e.Path))
	}

	return b.String()
}

// Unwrap provides compatibility with errors.Is and errors.As
func (e *RichError) Unwrap() error {
	return e.Err
}

// Format provides detailed error formatting for different verbosity levels
func (e *RichError) Format(f fmt.State, verb rune) {
	switch verb {
	case 'v':
		if f.Flag('+') {
			// Detailed format with suggestions
			fmt.Fprintf(f, "%s\n", e.Error()) //nolint:errcheck

			if len(e.Context) > 0 {
				fmt.Fprintf(f, "\nContext:\n") //nolint:errcheck
				for k, v := range e.Context {
					fmt.Fprintf(f, "  %s: %v\n", k, v) //nolint:errcheck
				}
			}

			if len(e.Suggestions) > 0 {
				fmt.Fprintf(f, "\nSuggestions:\n") //nolint:errcheck
				for i, s := range e.Suggestions {
					fmt.Fprintf(f, "  %d. %s\n", i+1, s) //nolint:errcheck
				}
			}
		} else {
			fmt.Fprintf(f, "%s", e.Error()) //nolint:errcheck
		}
	case 's':
		fmt.Fprintf(f, "%s", e.Error()) //nolint:errcheck
	default:
		fmt.Fprintf(f, "%s", e.Error()) //nolint:errcheck
	}
}

// New creates a new RichError with automatic source location
func New(errType ErrorType, op string, err error) *RichError {
	_, file, line, _ := runtime.Caller(1)
	return &RichError{
		Type:        errType,
		Op:          op,
		Err:         err,
		Context:     make(map[string]interface{}),
		Suggestions: []string{},
		Path:        file,
		Line:        line,
	}
}

// WithPath adds a file path to the error
func (e *RichError) WithPath(path string) *RichError {
	e.Path = path
	return e
}

// WithContext adds context information to the error
func (e *RichError) WithContext(key string, value interface{}) *RichError {
	e.Context[key] = value
	return e
}

// WithSuggestion adds a suggestion to the error
func (e *RichError) WithSuggestion(suggestion string, args ...interface{}) *RichError {
	if len(args) > 0 {
		suggestion = fmt.Sprintf(suggestion, args...)
	}
	e.Suggestions = append(e.Suggestions, suggestion)
	return e
}

// Is implements errors.Is interface
func (e *RichError) Is(target error) bool {
	if target == nil {
		return false
	}

	if e.Err != nil {
		return e.Err == target
	}

	t, ok := target.(*RichError)
	if !ok {
		return false
	}

	return e.Type == t.Type
}
