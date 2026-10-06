package templates

import (
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitConfigTOML_NameIsAlwaysAValueNeverStructure(t *testing.T) {
	tests := []struct {
		name, project, want string
	}{
		{name: "plain", project: "demo", want: "demo"},
		{name: "quote and newline cannot inject keys", project: "x\"\nevil = \"1", want: "x\"\nevil = \"1"},
		{name: "backslash and control characters", project: "a\\b\tc\x01", want: "a\\b\tc\x01"},
		{name: "comment-closing newline", project: "n\n[hooks]", want: "n\n[hooks]"},
		{name: "empty falls back to a valid default", project: "", want: "project"},
		{name: "blank falls back too", project: "  ", want: "project"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange / Act
			doc := InitConfigTOML(tt.project, nil)

			// Assert: the document parses to exactly name, version and presets.
			var parsed map[string]any
			require.NoError(t, toml.Unmarshal([]byte(doc), &parsed))
			assert.Equal(t, tt.want, parsed["name"])
			assert.ElementsMatch(t, []string{"version", "name", "presets"}, keysOf(parsed))
		})
	}
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
