package generator

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectPluginOutputs_MembersWithTheSamePluginName(t *testing.T) {
	// Arrange: two member roots whose [plugin] blocks share a name.
	dir := t.TempDir()
	writeDomainsFile(t, filepath.Join(dir, ".ai-rulez", "config.toml"),
		"version = \"5.0\"\nname = \"root\"\n\n[marketplace]\nname = \"acme\"\nmembers = [\"plugins/one\", \"plugins/two\"]\n")
	for _, member := range []string{"one", "two"} {
		writeDomainsFile(t, filepath.Join(dir, "plugins", member, ".ai-rulez", "config.toml"),
			"version = \"5.0\"\nname = \""+member+"\"\n\n[plugin]\nname = \"same\"\nversion = \"1.0.0\"\ndescription = \"d\"\nruntimes = [\"claude\"]\n")
	}
	gen := loadDomainsProjectUnvalidated(t, dir)

	// Act
	_, err := gen.collectPluginOutputs("")

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), `two plugins named "same"`)
	assert.Contains(t, err.Error(), "plugins/one")
	assert.Contains(t, err.Error(), "plugins/two")
}
