package generator

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const secretServerConfig = `version = "5.0"
name = "secret-check"
presets = ["claude"]
gitignore = %s

[[mcp_servers]]
name = "api"
command = "api-server"
args = ["--token", "ghp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"]
`

// unignoredSecretProject generates a project whose .mcp.json carries a literal
// secret while it is ignored, then turns gitignore off and drops .gitignore: the
// outputs are in sync, but generate would now refuse to write them.
func unignoredSecretProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(filepath.Join(configDir, "rules"), 0o755))
	writeConfig := func(gitignore string) {
		require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.toml"),
			[]byte(fmt.Sprintf(secretServerConfig, gitignore)), 0o644))
	}
	writeConfig("true")
	require.NoError(t, newProjectGenerator(t, dir).Generate("default"))
	require.FileExists(t, filepath.Join(dir, ".mcp.json"))
	writeConfig("false")
	require.NoError(t, os.Remove(filepath.Join(dir, ".gitignore")))
	return dir
}

func TestCheckAndDryRun_AgreeWithGenerateOnAnUnignoredSecret(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, gen *Generator)
	}{
		{
			name: "generate refuses",
			run: func(t *testing.T, gen *Generator) {
				t.Helper()
				err := gen.Generate("default")
				require.Error(t, err)
				assert.Contains(t, err.Error(), "contains secrets but is not gitignored: .mcp.json")
			},
		},
		{
			name: "check reports it blocked",
			run: func(t *testing.T, gen *Generator) {
				t.Helper()
				drift, err := gen.CheckDrift("default")
				require.NoError(t, err)
				assert.Contains(t, drift, Drift{Path: ".mcp.json", Kind: DriftBlocked})
			},
		},
		{
			name: "dry run marks it blocked",
			run: func(t *testing.T, gen *Generator) {
				t.Helper()
				lines, err := gen.DryRun("default")
				require.NoError(t, err)
				assert.Contains(t, lines, "blocked: .mcp.json ("+reasonSecretUnignored+")")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quietWarnings(t)
			// Arrange
			dir := unignoredSecretProject(t)
			gen := newProjectGenerator(t, dir)

			// Act + Assert
			tt.run(t, gen)
		})
	}
}
