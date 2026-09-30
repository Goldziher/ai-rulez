package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderOpenCodeScaffoldsMissingSource(t *testing.T) {
	m := &Manifest{Name: "test-plugin", Version: "1.2.3", SourceDir: t.TempDir()}

	outputs, err := renderOpenCode(m, "/out")
	require.NoError(t, err)
	require.Len(t, outputs, 2)
	assert.Equal(t, filepath.Join("/out", ".opencode", "plugins", "test-plugin.js"), outputs[0].Path)
	assert.Contains(t, string(outputs[0].RawContent), ".ai-rulez/opencode/index.js")

	// The scaffold must use the OpenCode v2 plugin API, not the v1 function
	// entrypoint that v2 refuses to run (#194).
	scaffold := string(outputs[0].RawContent)
	assert.Contains(t, scaffold, `from "@opencode/plugin"`)
	assert.Contains(t, scaffold, "Plugin.define")
	assert.Contains(t, scaffold, `id: "test-plugin"`)

	assert.Equal(t, filepath.Join("/out", "package.json"), outputs[1].Path)
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
	deps := pkg["dependencies"].(map[string]any)
	assert.Equal(t, "^2.0.20", deps["@opencode/plugin"], "v2 plugin package dependency")
}

func TestRenderOpenCodeBundlesContent(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "skills", "review")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: review\n---\nbody\n"), 0o644))
	agentPath := filepath.Join(root, "agents", "reviewer.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(agentPath), 0o755))
	require.NoError(t, os.WriteFile(agentPath, []byte("---\ndescription: x\n---\nbody\n"), 0o644))

	m := &Manifest{
		Name:      "test-plugin",
		Version:   "1.2.3",
		SourceDir: root,
		Skills:    []config.ContentFile{{Name: "review", Path: filepath.Join(skillDir, "SKILL.md")}},
		Agents:    []config.ContentFile{{Name: "reviewer", Path: agentPath}},
	}

	outputs, err := renderOpenCode(m, "/out")
	require.NoError(t, err)

	paths := make(map[string]bool, len(outputs))
	for _, o := range outputs {
		paths[filepath.ToSlash(o.Path)] = true
	}
	assert.True(t, paths["/out/.opencode/skills/review/SKILL.md"], "skill bundled under .opencode")
	assert.True(t, paths["/out/.opencode/agents/reviewer.md"], "agent bundled under .opencode")
}

func TestOpenCodePackageNameFallsBackWithoutGitHubRepository(t *testing.T) {
	assert.Equal(t, "opencode-example", openCodePackageName(&Manifest{Name: "example"}))
}
