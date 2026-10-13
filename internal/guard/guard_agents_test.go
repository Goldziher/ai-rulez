package guard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeFile(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func TestCheck_AgentsSourceProjects(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
	}{
		{"config in .agents", map[string]string{
			".agents/ai-rulez.toml":                      "version = \"5.0\"\n",
			".agents/.ai-rulez/.generated-manifest.json": `{"files":["CLAUDE.md"]}`,
		}},
		{"source key in the .ai-rulez config", map[string]string{
			".ai-rulez/config.toml":              "version = \"5.0\"\nsource = \"agents\"\n",
			".ai-rulez/.generated-manifest.json": `{"files":["CLAUDE.md"]}`,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			if real, err := filepath.EvalSymlinks(root); err == nil {
				root = real
			}
			for rel, body := range tt.files {
				writeFile(t, root, rel, body)
			}
			writeFile(t, root, "CLAUDE.md", "@AGENTS.md\n")
			writeFile(t, root, "AGENTS.md", "# Mine\n")
			edit := func(file string) Decision {
				return Check(strings.NewReader(`{"tool_name":"Edit","tool_input":{"file_path":"`+esc(filepath.Join(root, file))+`"}}`), root)
			}

			// Act
			generated := edit("CLAUDE.md")
			source := edit("AGENTS.md")

			// Assert
			assert.True(t, generated.Block, "the generated shim is guarded")
			assert.True(t, generated.Agents)
			assert.Contains(t, generated.Message(), "AGENTS.md or under .agents/")
			assert.NotContains(t, generated.Message(), "under .ai-rulez/")
			assert.False(t, source.Block, "AGENTS.md is the source and stays editable")
		})
	}
}
