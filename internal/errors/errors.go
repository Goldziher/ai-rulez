package errors

import (
	"fmt"
	"runtime"
	"strings"
)

type ErrorType int

const (
	ErrorTypeConfig ErrorType = iota
	ErrorTypeConfigNotFound
	ErrorTypeConfigInvalid
	ErrorTypeConfigSchema
	ErrorTypeConfigCircular

	ErrorTypeFileRead
	ErrorTypeFileWrite
	ErrorTypeFilePermission

	ErrorTypeTemplate
	ErrorTypeTemplateNotFound
	ErrorTypeTemplateSyntax

	ErrorTypeValidation
	ErrorTypeValidationRequired
	ErrorTypeValidationFormat

	ErrorTypeCommand
	ErrorTypeCommandArgument

	ErrorTypeGeneration
	ErrorTypeGenerationOutputNotFound

	ErrorTypeMCP

	ErrorTypeRemote
	ErrorTypeRemoteNetwork
	ErrorTypeRemoteSSRF
	ErrorTypeRemoteTimeout
	ErrorTypeRemoteHTTP
)

func (t ErrorType) String() string {
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
		ErrorTypeRemote:                   "remote",
		ErrorTypeRemoteNetwork:            "remote_network",
		ErrorTypeRemoteSSRF:               "remote_ssrf",
		ErrorTypeRemoteTimeout:            "remote_timeout",
		ErrorTypeRemoteHTTP:               "remote_http",
	}

	if str, ok := errorTypeStrings[t]; ok {
		return str
	}
	return "unknown"
}

type RichError struct {
	Type        ErrorType
	Op          string
	Path        string
	Err         error
	Context     map[string]interface{}
	Suggestions []string
	File        string
	Line        int
}

func (e *RichError) Error() string {
	var b strings.Builder

	if e.Op != "" {
		b.WriteString(e.Op)
		b.WriteString(": ")
	}

	if e.Err != nil {
		b.WriteString(e.Err.Error())
	}

	if e.Path != "" {
		b.WriteString(fmt.Sprintf(" (path: %s)", e.Path))
	}

	return b.String()
}

func (e *RichError) Unwrap() error {
	return e.Err
}

func (e *RichError) Format(f fmt.State, verb rune) {
	switch verb {
	case 'v':
		if f.Flag('+') {
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

func New(errType ErrorType, op string, err error) *RichError {
	_, file, line, _ := runtime.Caller(1)
	return &RichError{
		Type:        errType,
		Op:          op,
		Err:         err,
		Context:     make(map[string]interface{}),
		Suggestions: []string{},
		File:        file,
		Line:        line,
	}
}

func (e *RichError) WithPath(path string) *RichError {
	e.Path = path
	return e
}

func (e *RichError) WithContext(key string, value interface{}) *RichError {
	e.Context[key] = value
	return e
}

func (e *RichError) WithSuggestion(suggestion string, args ...interface{}) *RichError {
	if len(args) > 0 {
		suggestion = fmt.Sprintf(suggestion, args...)
	}
	e.Suggestions = append(e.Suggestions, suggestion)
	return e
}

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
