package plugin

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSONOutputKeepsHTMLCharacters(t *testing.T) {
	tests := []struct {
		name  string
		value map[string]string
		want  string
	}{
		{
			name:  "ampersand is not escaped",
			value: map[string]string{"description": "Tools & tips"},
			want:  "{\n  \"description\": \"Tools & tips\"\n}\n",
		},
		{
			name:  "angle brackets are not escaped",
			value: map[string]string{"description": "<plugin>"},
			want:  "{\n  \"description\": \"<plugin>\"\n}\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			out, err := jsonOutput("package.json", tt.value)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(out.RawContent))
		})
	}
}

func TestJSONOutputMatchesEncodingJSONShape(t *testing.T) {
	// Arrange
	value := map[string]any{"b": []int{1, 2}, "a": "x"}

	// Act
	out, err := jsonOutput("out.json", value)
	require.NoError(t, err)

	// Assert: valid JSON, two-space indent, trailing newline, keys sorted.
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(out.RawContent, &decoded))
	assert.Equal(t, "{\n  \"a\": \"x\",\n  \"b\": [\n    1,\n    2\n  ]\n}\n", string(out.RawContent))
}

func TestRenderOpenCodeKeepsAmpersandInDescription(t *testing.T) {
	// Arrange
	m := &Manifest{
		Name:        "test-plugin",
		Version:     "1.2.3",
		Description: "Tools & tips",
		SourceDir:   t.TempDir(),
	}

	// Act
	outputs, err := renderOpenCode(m, "/out")
	require.NoError(t, err)

	// Assert
	assert.Contains(t, string(outputs[1].RawContent), `"description": "Tools & tips"`)
	assert.NotContains(t, string(outputs[1].RawContent), `\u0026`)
}
