package crud

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/samber/oops"
)

// Reserved domain names that should not be used
var reservedDomainNames = map[string]bool{
	"rules":   true,
	"context": true,
	"skills":  true,
	"mcp":     true,
	".":       true,
	"..":      true,
}

// ValidateDomainName validates a domain name
// Rules: alphanumeric + underscore/hyphen, 1-50 chars, not reserved
func ValidateDomainName(name string) error {
	if name == "" {
		return oops.
			With("field", "domain_name").
			Hint("Domain name cannot be empty. Use a descriptive name like 'backend', 'frontend', or 'shared'.").
			Errorf("domain name is required")
	}

	if len(name) > 50 {
		return oops.
			With("field", "domain_name").
			With("length", len(name)).
			Hint("Domain name must be 50 characters or less.").
			Errorf("domain name too long: %d characters", len(name))
	}

	if reservedDomainNames[strings.ToLower(name)] {
		return oops.
			With("field", "domain_name").
			With("value", name).
			Hint(fmt.Sprintf("Reserved names: %v. Choose a different name.", getDomainReservedNames())).
			Errorf("domain name is reserved: %s", name)
	}

	// Match: alphanumeric, underscore, hyphen (must start and end with alphanumeric)
	pattern := regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*[a-zA-Z0-9]$|^[a-zA-Z0-9]$`)
	if !pattern.MatchString(name) {
		return oops.
			With("field", "domain_name").
			With("value", name).
			Hint("Domain names must: start and end with alphanumeric, contain only alphanumeric/underscore/hyphen. Examples: 'backend', 'frontend_api', 'shared-rules'.").
			Errorf("invalid domain name format: %s", name)
	}

	return nil
}

// ValidateFileName validates a file name for rules, context, or skills
// Rules: no path separators, valid filesystem name, 1-100 chars
func ValidateFileName(name string) error {
	if name == "" {
		return oops.
			With("field", "file_name").
			Hint("File name cannot be empty. Examples: 'my-rule', 'backend-context', 'api-client'.").
			Errorf("file name is required")
	}

	if len(name) > 100 {
		return oops.
			With("field", "file_name").
			With("length", len(name)).
			Hint("File name must be 100 characters or less.").
			Errorf("file name too long: %d characters", len(name))
	}

	// Check for path separators
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return oops.
			With("field", "file_name").
			With("value", name).
			Hint("File name cannot contain path separators (/ or \\).").
			Errorf("file name contains invalid path separators: %s", name)
	}

	// Check for invalid filesystem characters
	invalidChars := []string{":", "*", "?", "\"", "<", ">", "|", "\x00"}
	for _, char := range invalidChars {
		if strings.Contains(name, char) {
			return oops.
				With("field", "file_name").
				With("value", name).
				With("invalid_char", char).
				Hint("File name contains invalid filesystem character.").
				Errorf("file name contains invalid character %q: %s", char, name)
		}
	}

	// Check for reserved names
	if name == "SKILL.md" || name == ".." || name == "." {
		return oops.
			With("field", "file_name").
			With("value", name).
			Hint("This name is reserved. Choose a different file name.").
			Errorf("file name is reserved: %s", name)
	}

	return nil
}

// ValidateFileType validates a file type
func ValidateFileType(ftype string) error {
	validTypes := map[string]bool{
		"rules":   true,
		"context": true,
		"skills":  true,
	}

	if !validTypes[ftype] {
		return oops.
			With("field", "type").
			With("value", ftype).
			Hint("Valid types: rules, context, skills.").
			Errorf("invalid file type: %s", ftype)
	}

	return nil
}

// ValidatePriority validates a priority level
func ValidatePriority(priority string) error {
	validPriorities := map[string]bool{
		"critical": true,
		"high":     true,
		"medium":   true,
		"low":      true,
		"":         true, // Empty is allowed (defaults to medium)
	}

	if !validPriorities[priority] {
		return oops.
			With("field", "priority").
			With("value", priority).
			Hint("Valid priorities: critical, high, medium, low (or leave empty for medium).").
			Errorf("invalid priority level: %s", priority)
	}

	return nil
}

// ValidateIncludeSource validates an include source (git URL or local path)
func ValidateIncludeSource(source string) error {
	if source == "" {
		return oops.
			With("field", "source").
			Hint("Source must be a git URL (https://github.com/...) or local path.").
			Errorf("include source is required")
	}

	// Check if it's a git URL
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") || strings.HasPrefix(source, "git@") {
		return nil
	}

	// Check if it's a valid local path (relative or absolute)
	if _, err := os.Stat(source); err == nil {
		return nil
	}

	// If relative path doesn't exist, allow it (will be created)
	if !filepath.IsAbs(source) {
		return nil
	}

	return oops.
		With("field", "source").
		With("value", source).
		Hint("Source must be a valid git URL or existing/creatable local path.").
		Errorf("invalid include source: %s", source)
}

// ValidateDescription validates a description string
func ValidateDescription(desc string, fieldName string) error {
	if len(desc) > 500 {
		return oops.
			With("field", fieldName).
			With("length", len(desc)).
			Hint("Description must be 500 characters or less.").
			Errorf("%s too long: %d characters", fieldName, len(desc))
	}
	return nil
}

// Helper functions

func getDomainReservedNames() []string {
	names := make([]string, 0)
	for name := range reservedDomainNames {
		names = append(names, name)
	}
	return names
}
