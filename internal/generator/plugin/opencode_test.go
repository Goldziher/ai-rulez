package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderOpenCodeScaffoldsMissingSource(t *testing.T) {
	m := &Manifest{Name: "test-plugin", Version: "1.2.3", SourceDir: t.TempDir()}

	outputs, err := renderOpenCode(m, "/out")
	require.NoError(t, err)
	require.Len(t, outputs, 2)
	assert.Equal(t, "/out/.opencode/plugins/test-plugin.js", outputs[0].Path)
	assert.Contains(t, string(outputs[0].RawContent), ".ai-rulez/opencode/index.js")
	assert.Equal(t, "/out/package.json", outputs[1].Path)
}

func TestRenderOpenCodeCopiesAuthoredSource(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, openCodeSourcePath)
	require.NoError(t, os.MkdirAll(filepath.Dir(source), 0o755))
	require.NoError(t, os.WriteFile(source, []byte("export default async () => ({});\n"), 0o644))
	m := &Manifest{Name: "test-plugin", Version: "1.2.3", SourceDir: root}

	outputs, err := renderOpenCode(m, "/out")
	require.NoError(t, err)
	assert.Equal(t, "export default async () => ({});\n", string(outputs[0].RawContent))
}

func TestRenderOpenCodeGeneratesPackageFromMetadata(t *testing.T) {
	m := &Manifest{
		Name:       "test-plugin",
		Version:    "1.2.3",
		SourceDir:  t.TempDir(),
		Repository: "https://github.com/Xberg-IO/plugins",
		Homepage:   "https://xberg.io",
		License:    "MIT",
		Keywords:   []string{"documents"},
	}

	outputs, err := renderOpenCode(m, "/out")
	require.NoError(t, err)

	var pkg map[string]any
	require.NoError(t, json.Unmarshal(outputs[1].RawContent, &pkg))
	assert.Equal(t, "@xberg-io/opencode-test-plugin", pkg["name"])
	assert.Equal(t, ".opencode/plugins/test-plugin.js", pkg["main"])
	assert.Equal(t, "https://xberg.io", pkg["homepage"])
	assert.Equal(t, "MIT", pkg["license"])
	repository := pkg["repository"].(map[string]any)
	assert.Equal(t, "git", repository["type"])
	assert.Equal(t, "https://github.com/Xberg-IO/plugins", repository["url"])
}

func TestOpenCodePackageNameFallsBackWithoutGitHubRepository(t *testing.T) {
	assert.Equal(t, "opencode-example", openCodePackageName(&Manifest{Name: "example"}))
}
