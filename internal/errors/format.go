package errors

import (
	"errors"
	"fmt"
	"io"
	"strings"
)

// FormatError formats a RichError for display with color support
func FormatError(w io.Writer, err error, verbose bool, color bool) {
	if err == nil {
		return
	}
	
	var richErr *RichError
	ok := errors.As(err, &richErr)
	if !ok {
		// Fallback for non-rich errors
		fmt.Fprintf(w, "Error: %v\n", err) //nolint:errcheck
		return
	}
	
	// Color codes
	var (
		red    = "\033[31m"
		yellow = "\033[33m"
		cyan   = "\033[36m"
		reset  = "\033[0m"
		bold   = "\033[1m"
	)
	
	if !color {
		red = ""
		yellow = ""
		cyan = ""
		reset = ""
		bold = ""
	}
	
	// Primary error message
	fmt.Fprintf(w, "%s%sError:%s %s\n", bold, red, reset, richErr.Error()) //nolint:errcheck
	
	// Show context in verbose mode
	if verbose && len(richErr.Context) > 0 {
		fmt.Fprintf(w, "\n%sContext:%s\n", yellow, reset) //nolint:errcheck
		for k, v := range richErr.Context {
			// Format context values nicely
			switch val := v.(type) {
			case []string:
				if len(val) > 0 {
					fmt.Fprintf(w, "  %s: %s\n", k, strings.Join(val, " -> ")) //nolint:errcheck
				}
			case string:
				if val != "" {
					fmt.Fprintf(w, "  %s: %s\n", k, val) //nolint:errcheck
				}
			default:
				fmt.Fprintf(w, "  %s: %v\n", k, v) //nolint:errcheck
			}
		}
	}
	
	// Always show suggestions if available
	if len(richErr.Suggestions) > 0 {
		fmt.Fprintf(w, "\n%sSuggestions:%s\n", cyan, reset) //nolint:errcheck
		for i, suggestion := range richErr.Suggestions {
			fmt.Fprintf(w, "  %d. %s\n", i+1, suggestion) //nolint:errcheck
		}
	}
	
	// Show error type in verbose mode
	if verbose {
		fmt.Fprintf(w, "\n%sError Type:%s %s\n", yellow, reset, richErr.Type.String()) //nolint:errcheck
	}
}

// FormatErrorSimple formats just the error message without suggestions
func FormatErrorSimple(err error) string {
	if err == nil {
		return ""
	}
	
	var richErr *RichError
	ok := errors.As(err, &richErr)
	if !ok {
		return err.Error()
	}
	
	return richErr.Error()
}

// FormatErrorJSON formats a RichError as JSON for MCP or API responses
func FormatErrorJSON(err error) map[string]interface{} {
	if err == nil {
		return nil
	}
	
	var richErr *RichError
	ok := errors.As(err, &richErr)
	if !ok {
		return map[string]interface{}{
			"error": err.Error(),
			"type":  "unknown",
		}
	}
	
	result := map[string]interface{}{
		"error": richErr.Error(),
		"type":  richErr.Type.String(),
	}
	
	if richErr.Path != "" {
		result["path"] = richErr.Path
	}
	
	if len(richErr.Context) > 0 {
		result["context"] = richErr.Context
	}
	
	if len(richErr.Suggestions) > 0 {
		result["suggestions"] = richErr.Suggestions
	}
	
	return result
}

// ExtractRootCause extracts the root cause from a chain of wrapped errors
func ExtractRootCause(err error) error {
	if err == nil {
		return nil
	}
	
	for {
		unwrapper, ok := err.(interface{ Unwrap() error })
		if !ok {
			break
		}
		
		unwrapped := unwrapper.Unwrap()
		if unwrapped == nil {
			break
		}
		
		err = unwrapped
	}
	
	return err
}