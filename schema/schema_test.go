package schema_test

import (
	"strings"
	"testing"

	"github.com/samber/oops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/schema"
	"github.com/Goldziher/ai-rulez/testing/testutil"
)

func TestSchemaValidation(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid_minimal_config",
			yaml: `
metadata:
  name: "Test Project"
outputs:
  - file: "output.md"
`,
			wantErr: false,
		},
		{
			name: "valid_full_config",
			yaml: `
$schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v1.schema.json
metadata:
  name: "Test Project"
  version: "1.0.0"
  description: "A test project"
includes:
  - "other.yaml"
outputs:
  - file: "output.md"
  - file: "custom.md"
    template: "documentation"
  - file: "inline.md"
    template: |
      # {{.ProjectName}}
      {{range .Rules}}
      - {{.Name}}
      {{end}}
rules:
  - name: "Rule 1"
    priority: 10
    content: "Content 1"
  - name: "Rule 2"
    content: "Content 2"
`,
			wantErr: false,
		},
		{
			name: "missing_metadata",
			yaml: `
outputs:
  - file: "output.md"
`,
			wantErr: true,
			errMsg:  "metadata is required",
		},
		{
			name: "missing_metadata_name",
			yaml: `
metadata:
  version: "1.0.0"
outputs:
  - file: "output.md"
`,
			wantErr: true,
			errMsg:  "metadata: name is required",
		},
		{
			name: "missing_outputs",
			yaml: `
metadata:
  name: "Test"
`,
			wantErr: true,
			errMsg:  "outputs is required",
		},
		{
			name: "empty_outputs",
			yaml: `
metadata:
  name: "Test"
outputs: []
`,
			wantErr: true,
			errMsg:  "Array must have at least 1 items",
		},
		{
			name: "invalid_priority_string",
			yaml: `
metadata:
  name: "Test"
outputs:
  - file: "output.md"
rules:
  - name: "Rule"
    priority: "critical"
    content: "Content"
`,
			wantErr: true,
			errMsg:  "rules.0.priority",
		},
		{
			name: "invalid_priority_zero",
			yaml: `
metadata:
  name: "Test"
outputs:
  - file: "output.md"
rules:
  - name: "Rule"
    priority: 0
    content: "Content"
`,
			wantErr: true,
			errMsg:  "rules.0.priority",
		},
		{
			name: "invalid_priority_negative",
			yaml: `
metadata:
  name: "Test"
outputs:
  - file: "output.md"
rules:
  - name: "Rule"
    priority: -5
    content: "Content"
`,
			wantErr: true,
			errMsg:  "rules.0.priority",
		},
		{
			name: "missing_rule_name",
			yaml: `
metadata:
  name: "Test"
outputs:
  - file: "output.md"
rules:
  - content: "Content"
`,
			wantErr: true,
			errMsg:  "rules.0: name is required",
		},
		{
			name: "missing_rule_content",
			yaml: `
metadata:
  name: "Test"
outputs:
  - file: "output.md"
rules:
  - name: "Rule"
`,
			wantErr: true,
			errMsg:  "rules.0: content is required",
		},
		{
			name: "invalid_version_format",
			yaml: `
metadata:
  name: "Test"
  version: "v1.0"
outputs:
  - file: "output.md"
`,
			wantErr: true,
			errMsg:  "metadata.version",
		},
		{
			name: "file_reference_template",
			yaml: `
metadata:
  name: "Test"
outputs:
  - file: "output.md"
    template: "@templates/custom.tmpl"
`,
			wantErr: false,
		},
		{
			name: "invalid_template_format",
			yaml: `
metadata:
  name: "Test"
outputs:
  - file: "output.md"
    template: "123-invalid"
`,
			wantErr: true,
			errMsg:  "outputs.0.template",
		},
		{
			name: "additional_properties",
			yaml: `
metadata:
  name: "Test"
  unknown: "field"
outputs:
  - file: "output.md"
`,
			wantErr: true,
			errMsg:  "Additional property",
		},
		{
			name: "valid_sections",
			yaml: `
metadata:
  name: "Test"
outputs:
  - file: "output.md"
sections:
  - title: "Introduction"
    priority: 10
    content: "Welcome to the project"
  - title: "Usage"
    content: "How to use this"
`,
			wantErr: false,
		},
		{
			name: "section_missing_title",
			yaml: `
metadata:
  name: "Test"
outputs:
  - file: "output.md"
sections:
  - content: "Some content"
`,
			wantErr: true,
			errMsg:  "sections.0: title is required",
		},
		{
			name: "section_missing_content",
			yaml: `
metadata:
  name: "Test"
outputs:
  - file: "output.md"
sections:
  - title: "Introduction"
`,
			wantErr: true,
			errMsg:  "sections.0: content is required",
		},
		{
			name: "section_invalid_priority",
			yaml: `
metadata:
  name: "Test"
outputs:
  - file: "output.md"
sections:
  - title: "Introduction"
    priority: 0
    content: "Welcome"
`,
			wantErr: true,
			errMsg:  "sections.0.priority",
		},
		{
			name: "valid_remote_includes",
			yaml: `
metadata:
  name: "Test"
outputs:
  - file: "output.md"
includes:
  - "local-file.yaml"
  - "https://example.com/config.yaml"
  - "http://example.com/rules.yaml"
`,
			wantErr: false,
		},
		{
			name: "invalid_include_url_scheme",
			yaml: `
metadata:
  name: "Test"
outputs:
  - file: "output.md"
includes:
  - "ftp://example.com/config.yaml"
`,
			wantErr: true,
			errMsg:  "includes.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := schema.ValidateWithSchema([]byte(tt.yaml))

			if tt.wantErr {
				require.Error(t, err, "Expected validation to fail for test: %s", tt.name)
			} else {
				require.NoError(t, err, "Expected validation to pass for test: %s", tt.name)
			}
		})
	}
}

func TestSchemaValidationErrorMessages(t *testing.T) {
	tests := []struct {
		name           string
		yaml           string
		expectedErrors []string
		unexpectedText []string // ensure certain text does NOT appear
	}{
		{
			name: "missing_metadata_field",
			yaml: `
outputs:
  - file: "output.md"`,
			expectedErrors: []string{
				"'metadata': required field is missing",
				"metadata: required field is missing (expected object)",
			},
			unexpectedText: []string{
				"Property {property}",
				"does not match the schema",
			},
		},
		{
			name: "missing_metadata_name",
			yaml: `
metadata:
  version: "1.0.0"
outputs:
  - file: "output.md"`,
			expectedErrors: []string{
				"metadata.name: required field is missing",
			},
			unexpectedText: []string{
				"Property {property}",
			},
		},
		{
			name: "missing_outputs_field",
			yaml: `
metadata:
  name: "Test Project"`,
			expectedErrors: []string{
				"'outputs': required field is missing",
				"outputs: required field is missing (expected array)",
			},
			unexpectedText: []string{
				"Property {property}",
			},
		},
		{
			name: "empty_outputs_array",
			yaml: `
metadata:
  name: "Test Project"
outputs: []`,
			expectedErrors: []string{
				"outputs: must have at least 1 item",
			},
			unexpectedText: []string{
				"minItems",
				"Array must have",
			},
		},
		{
			name: "additional_property_in_metadata",
			yaml: `
metadata:
  name: "Test Project"
  unknown: "value"
  another_unknown: 123
outputs:
  - file: "output.md"`,
			expectedErrors: []string{
				"metadata.additionalProperties:",
				"'unknown'",
			},
			unexpectedText: []string{
				"schema is set to 'false'",
			},
		},
		{
			name: "missing_output_file",
			yaml: `
metadata:
  name: "Test Project"
outputs:
  - template: "custom"`,
			expectedErrors: []string{
				"outputs.0.file: required field is missing",
			},
		},
		{
			name: "invalid_priority_values",
			yaml: `
metadata:
  name: "Test Project"
outputs:
  - file: "output.md"
rules:
  - name: "Rule 1"
    priority: 0
    content: "Content"
  - name: "Rule 2"
    priority: -5
    content: "Content"`,
			expectedErrors: []string{
				"rules.0.priority",
				"rules.1.priority",
			},
		},
		{
			name: "missing_rule_fields",
			yaml: `
metadata:
  name: "Test Project"
outputs:
  - file: "output.md"
rules:
  - priority: 10`,
			expectedErrors: []string{
				"rules.0.name: required field is missing",
				"rules.0.content: required field is missing",
			},
		},
		{
			name: "invalid_section_priority",
			yaml: `
metadata:
  name: "Test Project"
outputs:
  - file: "output.md"
sections:
  - title: "Intro"
    priority: 0
    content: "Welcome"`,
			expectedErrors: []string{
				"sections.0.priority",
			},
		},
		{
			name: "complex_nested_errors",
			yaml: `
metadata:
  version: "invalid-version"
  extra_field: "not allowed"
outputs:
  - template: "test"
sections:
  - priority: 5
rules:
  - name: "Test"`,
			expectedErrors: []string{
				"metadata.name: required field is missing",
				"outputs.0.file: required field is missing",
				"sections.0.name: required field is missing",
				"sections.0.content: required field is missing",
				"rules.0.content: required field is missing",
			},
		},
		{
			name: "invalid_agent_configuration",
			yaml: `
metadata:
  name: "Test Project"
outputs:
  - file: "output.md"
agents:
  - name: "test-agent"
    priority: 0
    tools: ["invalid"]`,
			expectedErrors: []string{
				"agents.0.priority",
			},
		},
		{
			name: "invalid_include_url",
			yaml: `
metadata:
  name: "Test Project"
outputs:
  - file: "output.md"
includes:
  - "ftp://invalid.com/config.yaml"
  - "file:///etc/passwd"`,
			expectedErrors: []string{
				"includes.0",
				"includes.1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := schema.ValidateWithSchema([]byte(tt.yaml))
			require.Error(t, err, "Expected validation to fail")

			// Extract error messages
			var errorMessages []string
			if oopsErr, ok := oops.AsOops(err); ok {
				if errors, ok := oopsErr.Context()["errors"].([]string); ok {
					errorMessages = errors
				}
			}

			// Check that we got some error messages
			require.NotEmpty(t, errorMessages, "Expected to extract error messages from validation error")

			// Verify expected errors are present
			for _, expectedErr := range tt.expectedErrors {
				found := false
				for _, actualErr := range errorMessages {
					if strings.Contains(actualErr, expectedErr) {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected error message containing '%s' not found. Got errors: %v", expectedErr, errorMessages)
			}

			// Verify unwanted text is NOT present
			for _, unwantedText := range tt.unexpectedText {
				for _, actualErr := range errorMessages {
					assert.NotContains(t, actualErr, unwantedText, "Error message should not contain generic placeholder: %s", unwantedText)
				}
			}

			// Log actual errors for debugging
			t.Logf("Actual errors for %s:\n", tt.name)
			for _, err := range errorMessages {
				t.Logf("  %s\n", err)
			}
		})
	}
}

func TestSchemaValidationEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		wantErr  bool
		errCheck func(t *testing.T, err error)
	}{
		{
			name: "yaml_parse_error",
			yaml: `
metadata:
  name: "Test
  outputs:
    - file: "test.md"`,
			wantErr: true,
			errCheck: func(t *testing.T, err error) {
				assert.Contains(t, err.Error(), "parse YAML")
			},
		},
		{
			name:    "empty_yaml",
			yaml:    "",
			wantErr: true,
			errCheck: func(t *testing.T, err error) {
				assert.Contains(t, err.Error(), "validation failed")
			},
		},
		{
			name:    "only_whitespace",
			yaml:    "   \n  \t  \n  ",
			wantErr: true,
		},
		{
			name: "valid_with_comments",
			yaml: `
# This is a comment
metadata:
  name: "Test Project" # inline comment
outputs:
  - file: "output.md"
# Another comment`,
			wantErr: false,
		},
		{
			name: "valid_multiline_content",
			yaml: `
metadata:
  name: "Test Project"
outputs:
  - file: "output.md"
rules:
  - name: "Multi-line Rule"
    content: |
      This is a multi-line
      content block that spans
      multiple lines.`,
			wantErr: false,
		},
		{
			name: "valid_unicode_content",
			yaml: `
metadata:
  name: "测试项目 🚀"
outputs:
  - file: "output.md"
rules:
  - name: "Unicode Rule"
    content: "This contains émojis 😊 and special çharacters ñ"`,
			wantErr: false,
		},
		{
			name: "very_deep_nesting",
			yaml: `
metadata:
  name: "Test Project"
outputs:
  - file: "output.md"
    template: |
      {{range .Rules}}
        {{range .SubRules}}
          {{range .Items}}
            {{.Content}}
          {{end}}
        {{end}}
      {{end}}`,
			wantErr: false,
		},
		{
			name: "special_characters_in_strings",
			yaml: `
metadata:
  name: "Test\"Project'with special"
outputs:
  - file: "output.md"`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := schema.ValidateWithSchema([]byte(tt.yaml))

			if tt.wantErr {
				require.Error(t, err)
				if tt.errCheck != nil {
					tt.errCheck(t, err)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConvertYAMLToJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected any
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: nil,
		},
		{
			name:     "string input",
			input:    "test",
			expected: "test",
		},
		{
			name:     "integer input",
			input:    42,
			expected: 42,
		},
		{
			name: "map[any]any to map[string]any",
			input: map[any]any{
				"key1": "value1",
				"key2": 123,
				"key3": map[any]any{
					"nested": "value",
				},
			},
			expected: map[string]any{
				"key1": "value1",
				"key2": 123,
				"key3": map[string]any{
					"nested": "value",
				},
			},
		},
		{
			name: "slice of mixed types",
			input: []any{
				"string",
				42,
				map[any]any{"key": "value"},
			},
			expected: []any{
				"string",
				42,
				map[string]any{"key": "value"},
			},
		},
		{
			name: "complex nested structure",
			input: map[any]any{
				"metadata": map[any]any{
					"name":    "test",
					"version": "1.0.0",
				},
				"rules": []any{
					map[any]any{
						"name":     "rule1",
						"priority": 10,
					},
				},
			},
			expected: map[string]any{
				"metadata": map[string]any{
					"name":    "test",
					"version": "1.0.0",
				},
				"rules": []any{
					map[string]any{
						"name":     "rule1",
						"priority": 10,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := testutil.ConvertYAMLToJSON(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
