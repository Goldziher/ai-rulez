package schema_test

import (
	"strings"
	"testing"

	"github.com/samber/oops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/schema"
	"github.com/Goldziher/ai-rulez/tests/testutil"
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
  - path: "output.md"
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
  - path: "output.md"
  - path: "custom.md"
    template:
      type: builtin
      value: documentation
  - path: "inline.md"
    template:
      type: inline
      value: |
        # {{.ProjectName}}
        {{range .Rules}}
        - {{.Name}}
        {{end}}
rules:
  - name: "Rule 1"
    priority: critical
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
  - path: "output.md"
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
  - path: "output.md"
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
  - path: "output.md"
rules:
  - name: "Rule"
    priority: "invalid_priority"
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
  - path: "output.md"
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
  - path: "output.md"
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
  - path: "output.md"
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
  - path: "output.md"
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
  - path: "output.md"
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
  - path: "output.md"
    template:
      type: file
      value: templates/custom.tmpl
`,
			wantErr: false,
		},
		{
			name: "invalid_template_format",
			yaml: `
metadata:
  name: "Test"
outputs:
  - path: "output.md"
    template: 123
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
  - path: "output.md"
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
  - path: "output.md"
sections:
  - name: "Introduction"
    priority: critical
    content: "Welcome to the project"
  - name: "Usage"
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
  - path: "output.md"
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
  - path: "output.md"
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
  - path: "output.md"
sections:
  - name: "Introduction"
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
  - path: "output.md"
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
  - path: "output.md"
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
		unexpectedText []string
	}{
		{
			name: "missing_metadata_field",
			yaml: `
outputs:
  - path: "output.md"`,
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
  - path: "output.md"`,
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
				"outputs: required field is missing",
				"presets: required field is missing",
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
  - path: "output.md"`,
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
  - template:
      type: builtin
      value: custom`,
			expectedErrors: []string{
				"outputs.0.path: required field is missing",
			},
		},
		{
			name: "invalid_priority_values",
			yaml: `
metadata:
  name: "Test Project"
outputs:
  - path: "output.md"
rules:
  - name: "Rule 1"
    priority: 999
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
  - path: "output.md"
rules:
  - priority: critical`,
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
  - path: "output.md"
sections:
  - name: "Intro" 
    priority: 999
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
  - template:
      type: builtin
      value: test
sections:
  - priority: medium
rules:
  - name: "Test"`,
			expectedErrors: []string{
				"metadata.name: required field is missing",
				"outputs.0.path: required field is missing",
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
  - path: "output.md"
agents:
  - name: "test-agent"
    priority: minimal
    tools: ["invalid"]`,
			expectedErrors: []string{
				"agents.0.description: required field is missing",
			},
		},
		{
			name: "invalid_include_url",
			yaml: `
metadata:
  name: "Test Project"
outputs:
  - path: "output.md"
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

			var errorMessages []string
			if oopsErr, ok := oops.AsOops(err); ok {
				if errors, ok := oopsErr.Context()["errors"].([]string); ok {
					errorMessages = errors
				}
			}

			require.NotEmpty(t, errorMessages, "Expected to extract error messages from validation error")

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

			for _, unwantedText := range tt.unexpectedText {
				for _, actualErr := range errorMessages {
					assert.NotContains(t, actualErr, unwantedText, "Error message should not contain generic placeholder: %s", unwantedText)
				}
			}

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
    - path: "test.md"`,
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
  - path: "output.md"
# Another comment`,
			wantErr: false,
		},
		{
			name: "valid_multiline_content",
			yaml: `
metadata:
  name: "Test Project"
outputs:
  - path: "output.md"
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
  - path: "output.md"
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
  - path: "output.md"
    template:
      type: inline
      value: |
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
  - path: "output.md"`,
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
