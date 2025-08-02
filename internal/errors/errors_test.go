package errors

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestRichError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  *RichError
		want string
	}{
		{
			name: "basic error",
			err:  New(ErrorTypeConfig, "load config", fmt.Errorf("file not found")),
			want: "load config: file not found",
		},
		{
			name: "error with path",
			err:  New(ErrorTypeConfig, "load config", fmt.Errorf("file not found")).WithPath("/test/path"),
			want: "load config: file not found (path: /test/path)",
		},
		{
			name: "error without operation",
			err: &RichError{
				Type: ErrorTypeConfig,
				Err:  fmt.Errorf("simple error"),
			},
			want: "simple error",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("RichError.Error() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRichError_Format(t *testing.T) {
	err := New(ErrorTypeConfigNotFound, "find config", fmt.Errorf("not found")).
		WithPath("/test/dir").
		WithContext("search_dir", "/test/dir").
		WithSuggestion("Run 'ai-rulez init'").
		WithSuggestion("Check the directory")
	
	// Test basic format
	basic := fmt.Sprintf("%s", err)
	if !strings.Contains(basic, "find config: not found") {
		t.Errorf("Basic format missing error message: %s", basic)
	}
	
	// Test verbose format
	verbose := fmt.Sprintf("%+v", err)
	if !strings.Contains(verbose, "Context:") {
		t.Errorf("Verbose format missing context: %s", verbose)
	}
	if !strings.Contains(verbose, "Suggestions:") {
		t.Errorf("Verbose format missing suggestions: %s", verbose)
	}
	if !strings.Contains(verbose, "ai-rulez init") {
		t.Errorf("Verbose format missing specific suggestion: %s", verbose)
	}
}

func TestRichError_WithMethods(t *testing.T) {
	err := New(ErrorTypeConfig, "test", fmt.Errorf("base error"))
	
	// Test method chaining
	result := err.
		WithPath("/test/path").
		WithContext("key", "value").
		WithSuggestion("suggestion %s", "arg")
	
	if result.Path != "/test/path" {
		t.Errorf("WithPath() failed: got %s, want /test/path", result.Path)
	}
	
	if result.Context["key"] != "value" {
		t.Errorf("WithContext() failed: got %v, want value", result.Context["key"])
	}
	
	if len(result.Suggestions) != 1 || result.Suggestions[0] != "suggestion arg" {
		t.Errorf("WithSuggestion() failed: got %v, want [suggestion arg]", result.Suggestions)
	}
}

func TestErrorType_String(t *testing.T) {
	tests := []struct {
		errType ErrorType
		want    string
	}{
		{ErrorTypeConfig, "config"},
		{ErrorTypeConfigNotFound, "config_not_found"},
		{ErrorTypeTemplate, "template"},
		{ErrorTypeValidation, "validation"},
		{ErrorTypeMCP, "mcp"},
	}
	
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.errType.String(); got != tt.want {
				t.Errorf("ErrorType.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRichError_Unwrap(t *testing.T) {
	baseErr := fmt.Errorf("base error")
	richErr := New(ErrorTypeConfig, "test", baseErr)
	
	if !errors.Is(richErr.Unwrap(), baseErr) {
		t.Errorf("Unwrap() failed: got different error")
	}
}

func TestConfigNotFound(t *testing.T) {
	err := ConfigNotFound("/test/dir")
	
	if err.Type != ErrorTypeConfigNotFound {
		t.Errorf("ConfigNotFound() type = %v, want %v", err.Type, ErrorTypeConfigNotFound)
	}
	
	if err.Path != "/test/dir" {
		t.Errorf("ConfigNotFound() path = %v, want /test/dir", err.Path)
	}
	
	if len(err.Suggestions) == 0 {
		t.Error("ConfigNotFound() should have suggestions")
	}
	
	// Check for specific suggestions
	suggestions := strings.Join(err.Suggestions, " ")
	if !strings.Contains(suggestions, "ai-rulez init") {
		t.Error("ConfigNotFound() should suggest running init")
	}
}

func TestCircularInclude(t *testing.T) {
	chain := []string{"a.yaml", "b.yaml", "c.yaml", "a.yaml"}
	err := CircularInclude(chain)
	
	if err.Type != ErrorTypeConfigCircular {
		t.Errorf("CircularInclude() type = %v, want %v", err.Type, ErrorTypeConfigCircular)
	}
	
	if err.Context["include_chain"] == nil {
		t.Error("CircularInclude() should include the chain in context")
	}
	
	if len(err.Suggestions) == 0 {
		t.Error("CircularInclude() should have suggestions")
	}
}