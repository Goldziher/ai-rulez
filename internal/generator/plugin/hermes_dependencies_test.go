package plugin

import (
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestRenderHermesDependencies(t *testing.T) {
	dependencies := []string{
		"example-tool[cli]>=0.1,<1",
		`demo; platform_system == "Linux" and implementation_name != 'pypy'`,
		`demo>=1.0,!=1.5.*; os_name == "nt" or sys_platform == 'win32'`,
	}
	tests := []struct {
		name   string
		hermes *config.HermesExtras
	}{
		{name: "omitted"},
		{name: "empty list", hermes: &config.HermesExtras{Dependencies: []string{}}},
		{name: "requirements", hermes: &config.HermesExtras{Dependencies: dependencies}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Manifest{Name: "example", Version: "1.2.3", SourceDir: t.TempDir(), Hermes: tt.hermes}
			outputs, err := renderHermes(m, t.TempDir())
			require.NoError(t, err)
			require.Len(t, outputs, 7)
			var project struct {
				Project map[string]any `toml:"project"`
			}
			require.NoError(t, toml.Unmarshal(outputs[3].RawContent, &project))
			for _, index := range []int{2, 6} {
				var manifest map[string]any
				require.NoError(t, yaml.Unmarshal(outputs[index].RawContent, &manifest))
				if tt.name == "requirements" {
					assert.Equal(t, dependencies, stringValues(t, manifest["python_dependencies"]))
					assert.Equal(t, dependencies, stringValues(t, project.Project["dependencies"]))
				} else {
					assert.NotContains(t, manifest, "python_dependencies")
					assert.NotContains(t, project.Project, "dependencies")
				}
			}
			assert.Equal(t, "example-hermes-plugin", project.Project["name"])
			assert.Contains(t, string(outputs[3].RawContent), "hermes_agent.plugins")
			assert.Contains(t, string(outputs[3].RawContent), "example_hermes_plugin")
			assert.Equal(t, outputs[1].RawContent, outputs[5].RawContent)
		})
	}
}

func stringValues(t *testing.T, value any) []string {
	t.Helper()
	values, ok := value.([]any)
	require.True(t, ok, "expected string array, got %T", value)
	result := make([]string, len(values))
	for i, v := range values {
		s, ok := v.(string)
		require.True(t, ok, "expected string, got %T", v)
		result[i] = s
	}
	return result
}
