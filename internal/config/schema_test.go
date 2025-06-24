package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai_rules/internal/config"
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
$schema: https://github.com/Goldziher/ai_rules/schema/ai-rules-v1.schema.json
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := config.ValidateWithSchema([]byte(tt.yaml))

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConvertYAMLToJSON(t *testing.T) {
	input := map[any]any{
		"string": "value",
		"number": 42,
		"nested": map[any]any{
			"key": "value",
		},
		"array": []any{
			"item1",
			map[any]any{
				"nested": "value",
			},
		},
	}

	result := config.ConvertYAMLToJSON(input)

	// Check that it's now a map[string]any
	m, ok := result.(map[string]any)
	require.True(t, ok)

	// Check values
	assert.Equal(t, "value", m["string"])
	assert.Equal(t, 42, m["number"])

	// Check nested map
	nested, ok := m["nested"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "value", nested["key"])

	// Check array
	arr, ok := m["array"].([]any)
	require.True(t, ok)
	assert.Equal(t, "item1", arr[0])

	// Check nested map in array
	arrNested, ok := arr[1].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "value", arrNested["nested"])
}
