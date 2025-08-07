package errors

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestRichError_FullFormatting tests the complete formatting of rich errors
func TestRichError_FullFormatting(t *testing.T) {
	t.Parallel()

	// Create a rich error with all fields
	err := New(ErrorTypeConfig, "test operation", fmt.Errorf("base error")).
		WithPath("/path/to/file").
		WithContext("key1", "value1").
		WithContext("key2", "value2").
		WithSuggestion("Try this first").
		WithSuggestion("Or try this")

	// Test basic error message
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "test operation")
	
	// Test formatted output
	var buf bytes.Buffer
	FormatError(&buf, err, true, false)
	formatted := buf.String()
	assert.NotEmpty(t, formatted)
	assert.Contains(t, formatted, "value1")
	assert.Contains(t, formatted, "value2")
}

// TestRichError_NilFields tests handling of nil/empty fields
func TestRichError_NilFields(t *testing.T) {
	t.Parallel()

	// Error with no base error
	err := New(ErrorTypeTemplate, "operation", nil)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "operation")

	// Error with empty operation
	err = New(ErrorTypeValidation, "", fmt.Errorf("error"))
	assert.Contains(t, err.Error(), "error")
}

// TestErrorType_AllTypes tests all error type strings
func TestErrorType_AllTypes(t *testing.T) {
	t.Parallel()

	types := []ErrorType{
		ErrorTypeConfig,
		ErrorTypeConfigNotFound,
		ErrorTypeConfigInvalid,
		ErrorTypeConfigSchema,
		ErrorTypeTemplate,
		ErrorTypeTemplateNotFound,
		ErrorTypeFileRead,
		ErrorTypeFileWrite,
		ErrorTypeValidation,
		ErrorTypeGeneration,
		ErrorTypeMCP,
		ErrorType(999), // Unknown type
	}

	for _, errType := range types {
		t.Run(fmt.Sprintf("type_%d", errType), func(t *testing.T) {
			str := errType.String()
			assert.NotEmpty(t, str)
		})
	}
}

// TestConfigNotFound_Whitebox tests ConfigNotFound error creation details
func TestConfigNotFound_Whitebox(t *testing.T) {
	t.Parallel()

	err := ConfigNotFound("/path/to/config")
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "config")
}

// TestConfigParse tests ConfigParse error creation
func TestConfigParse(t *testing.T) {
	t.Parallel()

	baseErr := fmt.Errorf("unexpected character at line 5")
	err := ConfigParse("config.yaml", baseErr)
	
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "parse")
}

// TestSchemaValidation tests SchemaValidation error creation
func TestSchemaValidation(t *testing.T) {
	t.Parallel()

	validationErrors := []string{
		"metadata.name is required",
		"outputs must have at least 1 item",
		"rules[0].content is required",
	}

	err := SchemaValidation("test.yaml", validationErrors)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "validation")
}

// TestCircularInclude_Whitebox tests CircularInclude error creation details
func TestCircularInclude_Whitebox(t *testing.T) {
	t.Parallel()

	chain := []string{"a.yaml", "b.yaml", "c.yaml", "a.yaml"}
	err := CircularInclude(chain)
	
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "circular")
}

// TestTemplateNotFound tests TemplateNotFound error creation
func TestTemplateNotFound(t *testing.T) {
	t.Parallel()

	err := TemplateNotFound("custom-template", []string{"template1", "template2"})
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "template")
}

// TestTemplateExecute tests TemplateExecute error creation
func TestTemplateExecute(t *testing.T) {
	t.Parallel()

	baseErr := fmt.Errorf("undefined variable: .NonExistent")
	err := TemplateExecute("output.md", baseErr)
	
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "template")
}

// TestFileRead tests FileRead error creation
func TestFileRead(t *testing.T) {
	t.Parallel()

	baseErr := fmt.Errorf("permission denied")
	err := FileRead("/protected/file", baseErr)
	
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "read")
}

// TestFileWrite tests FileWrite error creation
func TestFileWrite(t *testing.T) {
	t.Parallel()

	baseErr := fmt.Errorf("disk full")
	err := FileWrite("/output/file", baseErr)
	
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "write")
}

// TestValidationRequired tests ValidationRequired error creation
func TestValidationRequired(t *testing.T) {
	t.Parallel()

	err := ValidationRequired("metadata.name", "configuration file")
	
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "required")
}

// TestMCPError tests MCPError creation
func TestMCPError(t *testing.T) {
	t.Parallel()

	baseErr := fmt.Errorf("connection refused")
	err := MCPError("connect to server", baseErr)
	
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "connection")
}

// TestRichError_Unwrap_Whitebox tests error unwrapping internals
func TestRichError_Unwrap_Whitebox(t *testing.T) {
	t.Parallel()

	baseErr := fmt.Errorf("base error")
	richErr := New(ErrorTypeConfig, "operation", baseErr)
	
	unwrapped := richErr.Unwrap()
	assert.Equal(t, baseErr, unwrapped)

	// Test with nil base error
	richErr = New(ErrorTypeConfig, "operation", nil)
	unwrapped = richErr.Unwrap()
	assert.Nil(t, unwrapped)
}

// TestRichError_ChainedContext tests adding multiple contexts
func TestRichError_ChainedContext(t *testing.T) {
	t.Parallel()

	err := New(ErrorTypeConfig, "operation", fmt.Errorf("error")).
		WithContext("file", "test.yaml").
		WithContext("line", "42").
		WithContext("column", "15")

	var buf bytes.Buffer
	FormatError(&buf, err, true, false)
	formatted := buf.String()
	assert.Contains(t, formatted, "test.yaml")
	assert.Contains(t, formatted, "42")
	assert.Contains(t, formatted, "15")
}

// TestRichError_ChainedSuggestions tests adding multiple suggestions
func TestRichError_ChainedSuggestions(t *testing.T) {
	t.Parallel()

	err := New(ErrorTypeConfig, "operation", fmt.Errorf("error")).
		WithSuggestion("First suggestion").
		WithSuggestion("Second suggestion").
		WithSuggestion("Third suggestion")

	var buf bytes.Buffer
	FormatError(&buf, err, true, false)
	formatted := buf.String()
	assert.Contains(t, formatted, "First suggestion")
	assert.Contains(t, formatted, "Second suggestion")
	assert.Contains(t, formatted, "Third suggestion")
}

// TestRichError_ComplexScenario tests a complex error scenario
func TestRichError_ComplexScenario(t *testing.T) {
	t.Parallel()

	// Simulate a complex error from config loading
	baseErr := fmt.Errorf("yaml: line 10: found character that cannot start any token")
	
	err := New(ErrorTypeConfigInvalid, "parse YAML", baseErr).
		WithPath("/project/ai_rulez.yaml").
		WithContext("line", "10").
		WithContext("error_type", "syntax").
		WithSuggestion("Check for tabs instead of spaces").
		WithSuggestion("Ensure proper YAML indentation").
		WithSuggestion("Validate special characters are properly escaped")

	// Test error message
	errMsg := err.Error()
	assert.Contains(t, errMsg, "parse")
	
	// Test formatted output
	var buf bytes.Buffer
	FormatError(&buf, err, true, false)
	formatted := buf.String()
	assert.Contains(t, formatted, "ai_rulez.yaml")
	assert.Contains(t, formatted, "10")
}

// TestRichError_EmptyError tests creating error without base error
func TestRichError_EmptyError(t *testing.T) {
	t.Parallel()

	err := New(ErrorTypeValidation, "validation failed", nil).
		WithContext("reason", "missing required field")

	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "validation")
	
	var buf bytes.Buffer
	FormatError(&buf, err, true, false)
	formatted := buf.String()
	assert.Contains(t, formatted, "missing required field")
}

// TestFormatErrorJSON tests JSON formatting
func TestFormatErrorJSON(t *testing.T) {
	t.Parallel()

	err := New(ErrorTypeConfig, "test", fmt.Errorf("base")).
		WithPath("/test").
		WithContext("key", "value")

	jsonData := FormatErrorJSON(err)
	assert.NotNil(t, jsonData)
	assert.Contains(t, jsonData, "error")
	assert.Contains(t, jsonData, "type")
}

// TestFormatErrorSimple tests simple formatting
func TestFormatErrorSimple(t *testing.T) {
	t.Parallel()

	err := New(ErrorTypeConfig, "test", fmt.Errorf("base"))
	simple := FormatErrorSimple(err)
	assert.NotEmpty(t, simple)
	assert.Contains(t, simple, "test")
}

// TestTemplateParse tests TemplateParse error creation
func TestTemplateParse(t *testing.T) {
	t.Parallel()

	baseErr := fmt.Errorf("unexpected EOF")
	err := TemplateParse("template.tmpl", baseErr)
	
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "parse")
}