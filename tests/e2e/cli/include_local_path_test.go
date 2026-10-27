package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/tests/e2e/testutil"
)

func includeRule(title, id, marker string) string {
	return "---\ntype: Rule\ntitle: " + title + "\nx-ai-rulez:\n  kind: rule\n  id: " + id +
		"\n---\n\n# " + title + "\n\n" + marker + "\n"
}

// #510: a local include honors `path` and fails when it does not exist.
func TestLocalIncludeHonorsPath(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		wantExit   int
		wantMarker string
		wantErr    string
	}{
		{name: "existing subdirectory", path: "modules/x", wantMarker: "MODULE_X"},
		{name: "no path loads the source root", path: "", wantMarker: "SOURCE_ROOT"},
		{name: "missing subdirectory", path: "modules/does-not-exist", wantExit: 1, wantErr: "does-not-exist"},
		{name: "path climbing out of the source", path: "../outside", wantExit: 1, wantErr: ".."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := testutil.CreateTempDir(t)
			gitInit(t, dir)
			writeRel(t, dir, "src/rules/root.md", includeRule("Root", "root", "SOURCE_ROOT"))
			writeRel(t, dir, "src/modules/x/rules/mod.md", includeRule("Mod", "mod", "MODULE_X"))
			cfg := "version = \"5.0\"\nname = \"i\"\ngitignore = false\npresets = [\"claude\"]\n\n" +
				"[[includes]]\nname = \"x\"\nsource = \"./src\"\n"
			if tt.path != "" {
				cfg += "path = \"" + tt.path + "\"\n"
			}
			writeRel(t, dir, ".ai-rulez/config.toml", cfg)

			// Act
			res := testutil.RunCLI(t, dir, "generate", "--yes", "-q")

			// Assert
			require.Equal(t, tt.wantExit, res.ExitCode, res.Stdout+res.Stderr)
			if tt.wantErr != "" {
				assert.Contains(t, res.Stdout+res.Stderr, tt.wantErr)
				return
			}
			agents := testutil.ReadFile(t, filepath.Join(dir, "AGENTS.md"))
			assert.Contains(t, agents, tt.wantMarker)
			for _, other := range []string{"SOURCE_ROOT", "MODULE_X"} {
				if other != tt.wantMarker {
					assert.NotContains(t, agents, other)
				}
			}
		})
	}
}

// #510: validate reports a local include whose path does not exist.
func TestLocalIncludeMissingPathFailsValidate(t *testing.T) {
	dir := testutil.CreateTempDir(t)
	writeRel(t, dir, "src/rules/root.md", includeRule("Root", "root", "SOURCE_ROOT"))
	writeRel(t, dir, ".ai-rulez/config.toml", "version = \"5.0\"\nname = \"i\"\npresets = [\"claude\"]\n\n"+
		"[[includes]]\nname = \"x\"\nsource = \"./src\"\npath = \"modules/nope\"\n")

	res := testutil.RunCLI(t, dir, "validate")

	assert.NotEqual(t, 0, res.ExitCode, res.Stdout+res.Stderr)
	assert.Contains(t, res.Stdout+res.Stderr, "modules/nope")
}

func writeRel(t *testing.T, dir, rel, content string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
}
