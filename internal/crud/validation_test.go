package crud_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/stretchr/testify/assert"
)

// TestValidateDomainName tests domain name validation
func TestValidateDomainName(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		shouldErr bool
	}{
		// Valid cases
		{"single letter", "a", false},
		{"single digit", "1", false},
		{"simple name", "backend", false},
		{"with underscore", "backend_api", false},
		{"with hyphen", "backend-api", false},
		{"mixed valid chars", "backend_api-v2", false},
		{"long valid name (50 chars)", "backend_api_v2_domain_name_that_is_max_50_char_ok", false},

		// Invalid cases - empty
		{"empty string", "", true},

		// Invalid cases - too long
		{"too long (51 chars)", "a_very_long_domain_name_that_exceeds_the_fifty_character_limit", true},

		// Invalid cases - reserved names
		{"reserved: rules", "rules", true},
		{"reserved: context", "context", true},
		{"reserved: skills", "skills", true},
		{"reserved: mcp", "mcp", true},
		{"reserved: .", ".", true},
		{"reserved: ..", "..", true},

		// Invalid cases - bad format
		{"starts with hyphen", "-backend", true},
		{"ends with hyphen", "backend-", true},
		{"starts with underscore", "_backend", true},
		{"ends with underscore", "backend_", true},
		{"contains space", "backend api", true},
		{"contains dot", "backend.api", true},
		{"contains special char", "backend@api", true},
		{"contains slash", "backend/api", true},
		{"case insensitive reserved", "RULES", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := crud.ValidateDomainName(tt.input)
			if tt.shouldErr {
				assert.Error(t, err, "expected error for input: %q", tt.input)
			} else {
				assert.NoError(t, err, "unexpected error for input: %q", tt.input)
			}
		})
	}
}

// TestValidateFileName tests file name validation
func TestValidateFileName(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		shouldErr bool
	}{
		// Valid cases
		{"simple name", "my-rule", false},
		{"with underscore", "my_rule", false},
		{"mixed chars", "my-rule_v2", false},
		{"long name", "very-long-file-name-for-rule-valid-one-hundred-limit", false},

		// Invalid cases - empty
		{"empty string", "", true},

		// Invalid cases - too long
		{"too long (101+ chars)", "a_very_long_file_name_that_is_way_too_long_and_exceeds_the_maximum_allowed_length_for_file_names_in_the_system", true},

		// Invalid cases - path separators
		{"contains forward slash", "my/rule", true},
		{"contains backslash", "my\\rule", true},

		// Invalid cases - invalid filesystem characters
		{"contains colon", "my:rule", true},
		{"contains asterisk", "my*rule", true},
		{"contains question mark", "my?rule", true},
		{"contains quote", "my\"rule", true},
		{"contains less than", "my<rule", true},
		{"contains greater than", "my>rule", true},
		{"contains pipe", "my|rule", true},
		{"contains null byte", "my\x00rule", true},

		// Invalid cases - reserved names
		{"reserved: SKILL.md", "SKILL.md", true},
		{"reserved: .", ".", true},
		{"reserved: ..", "..", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := crud.ValidateFileName(tt.input)
			if tt.shouldErr {
				assert.Error(t, err, "expected error for input: %q", tt.input)
			} else {
				assert.NoError(t, err, "unexpected error for input: %q", tt.input)
			}
		})
	}
}

// TestValidateFileType tests file type validation
func TestValidateFileType(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		shouldErr bool
	}{
		// Valid cases
		{"rules", "rules", false},
		{"context", "context", false},
		{"skills", "skills", false},

		// Invalid cases
		{"empty string", "", true},
		{"rule (singular)", "rule", true},
		{"invalid type", "invalid", true},
		{"mcp", "mcp", true},
		{"RULES (uppercase)", "RULES", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := crud.ValidateFileType(tt.input)
			if tt.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidatePriority tests priority validation
func TestValidatePriority(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		shouldErr bool
	}{
		// Valid cases
		{"critical", "critical", false},
		{"high", "high", false},
		{"medium", "medium", false},
		{"low", "low", false},
		{"empty (default)", "", false},

		// Invalid cases
		{"invalid priority", "urgent", true},
		{"uppercase", "CRITICAL", true},
		{"partial match", "crit", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := crud.ValidatePriority(tt.input)
			if tt.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateIncludeSource tests include source validation
func TestValidateIncludeSource(t *testing.T) {
	// Create a non-existent absolute path that works on both Unix and Windows
	var nonexistentAbsPath string
	if runtime.GOOS == "windows" {
		nonexistentAbsPath = filepath.VolumeName(os.TempDir()) + `\nonexistent\absolute\path`
	} else {
		nonexistentAbsPath = "/nonexistent/absolute/path"
	}

	tests := []struct {
		name      string
		input     string
		shouldErr bool
	}{
		// Valid git URLs
		{"https git URL", "https://github.com/user/repo", false},
		{"https with .git suffix", "https://github.com/user/repo.git", false},
		{"http git URL", "http://github.com/user/repo.git", false},
		{"git SSH URL", "git@github.com:user/repo.git", false},

		// Invalid cases
		{"empty string", "", true},
		// Note: relative paths that don't exist are allowed (they can be created later)
		{"relative path allowed", "./relative/path", false},
		{"absolute path not found", nonexistentAbsPath, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := crud.ValidateIncludeSource(tt.input)
			if tt.shouldErr {
				assert.Error(t, err, "expected error for input: %q", tt.input)
			} else {
				assert.NoError(t, err, "unexpected error for input: %q", tt.input)
			}
		})
	}
}

// TestValidateDescription tests description validation
func TestValidateDescription(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		fieldName string
		shouldErr bool
	}{
		// Valid cases
		{"empty string", "", "description", false},
		{"short description", "A short description", "description", false},
		{"long but valid description", "This is a much longer description that provides more detail about what we are describing but stays within the character limit of five hundred characters total.", "description", false},
		{"max length", string(make([]byte, 500)), "description", false},

		// Invalid cases
		{"too long (501 chars)", string(make([]byte, 501)), "description", true},
		{"way too long (1000 chars)", string(make([]byte, 1000)), "description", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := crud.ValidateDescription(tt.input, tt.fieldName)
			if tt.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
