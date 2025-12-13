package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple name",
			input:    "Simple Name",
			expected: "simple-name",
		},
		{
			name:     "name with special chars",
			input:    "Name!@#$%With^&*Special()Chars",
			expected: "namewithspecialchars",
		},
		{
			name:     "name with multiple spaces",
			input:    "Name   With    Spaces",
			expected: "name-with-spaces",
		},
		{
			name:     "name with hyphens",
			input:    "Name-With-Hyphens",
			expected: "name-with-hyphens",
		},
		{
			name:     "name with underscores",
			input:    "Name_With_Underscores",
			expected: "name_with_underscores",
		},
		{
			name:     "name with consecutive hyphens",
			input:    "Name---With---Hyphens",
			expected: "name-with-hyphens",
		},
		{
			name:     "leading and trailing hyphens",
			input:    "-Name-",
			expected: "name",
		},
		{
			name:     "empty name",
			input:    "",
			expected: "unnamed",
		},
		{
			name:     "only special chars",
			input:    "!@#$%^&*()",
			expected: "unnamed",
		},
		{
			name:     "unicode characters",
			input:    "Café ☕ Rules",
			expected: "caf-rules",
		},
		{
			name:     "mixed case with numbers",
			input:    "Rule123ABC",
			expected: "rule123abc",
		},
		{
			name:     "spaces and underscores mixed",
			input:    "my_test rule",
			expected: "my_test-rule",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
