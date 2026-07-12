package plugin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderHermesScaffoldsMissingSource(t *testing.T) {
	m := &Manifest{Name: "example", Version: "1.2.3", SourceDir: t.TempDir()}
	outputs, err := renderHermes(m, "/out")
	require.NoError(t, err)
	require.Len(t, outputs, 7)
	assert.Equal(t, "/out/.hermes/plugins/example/__init__.py", outputs[0].Path)
	assert.Contains(t, string(outputs[0].RawContent), "from .hermes import register")
	assert.Contains(t, string(outputs[1].RawContent), ".ai-rulez/hermes/index.py")
	assert.Contains(t, string(outputs[2].RawContent), `kind: "standalone"`)
	assert.Equal(t, "/out/.hermes/package/pyproject.toml", outputs[3].Path)
	assert.Contains(t, string(outputs[3].RawContent), `name = 'example-hermes-plugin'`)
	assert.Equal(t, "/out/.hermes/package/src/example_hermes_plugin/__init__.py", outputs[4].Path)
}

func TestRenderHermesCopiesAuthoredSourceAndContent(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, hermesSourcePath)
	require.NoError(t, os.MkdirAll(filepath.Dir(source), 0o755))
	require.NoError(t, os.WriteFile(source, []byte("def register(ctx):\n    pass\n"), 0o644))
	m := &Manifest{Name: "example", Version: "1.2.3", SourceDir: root}
	outputs, err := renderHermes(m, root)
	require.NoError(t, err)
	assert.Equal(t, "def register(ctx):\n    pass\n", string(outputs[1].RawContent))
}

func TestHermesPyprojectNormalizesPrereleaseVersion(t *testing.T) {
	project, err := hermesPyproject(&Manifest{Name: "basemind", Version: "1.2.3-rc.4"}, "basemind_hermes_plugin")
	require.NoError(t, err)
	assert.Contains(t, string(project), `name = 'basemind-hermes-plugin'`)
	assert.Contains(t, string(project), `version = '1.2.3rc4'`)
}

func TestHermesPyprojectEscapesMetadata(t *testing.T) {
	project, err := hermesPyproject(
		&Manifest{Name: "example.plugin", Version: "1.2.3", Description: "line one\nline \\\"two\\\""},
		"example_plugin_hermes_plugin",
	)
	require.NoError(t, err)
	assert.Contains(t, string(project), `'example.plugin' = 'example_plugin_hermes_plugin'`)
}
