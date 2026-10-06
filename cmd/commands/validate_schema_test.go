package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/samber/oops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateConfigFile_SchemaErrorSuggestsTheKnownKey(t *testing.T) {
	// Arrange
	root := t.TempDir()
	path := filepath.Join(root, ".ai-rulez", "config.toml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("version = \"4.0\"\nname = \"p\"\npresets = [\"claude\"]\ngitignor = true\n"), 0o644))

	// Act
	_, err := validateConfigFile(path)

	// Assert
	require.Error(t, err)
	oopsErr, ok := oops.AsOops(err)
	require.True(t, ok)
	lines, _ := oopsErr.Context()["errors"].([]string)
	require.Len(t, lines, 1)
	assert.Contains(t, lines[0], `unknown key "gitignor"`)
	assert.Contains(t, lines[0], `did you mean "gitignore"`)
	assert.NotContains(t, lines[0], "- -")
	assert.NotContains(t, oopsErr.Hint(), "Run 'ai-rulez validate'")
}
