package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/opencodev1"
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

	// The scaffold must default-export a v2 { id, setup } definition, not the
	// v1 function entrypoint that v2 refuses to run (#194). It must not import
	// @opencode/plugin at runtime: OpenCode does not install dependencies for
	// local plugins, so the import would fail to resolve.
	scaffold := string(outputs[0].RawContent)
	assert.Contains(t, scaffold, "export default {")
	assert.Contains(t, scaffold, `id: "test-plugin"`)
	assert.Contains(t, scaffold, "async setup(ctx)")
	assert.NotContains(t, scaffold, `from "@opencode/plugin"`)
	assert.NotContains(t, scaffold, "import { registerBundledContent }", "no bundled content, no helper import")

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

func TestRenderOpenCodeWarnsOnV1AuthoredSource(t *testing.T) {
	tests := []struct {
		name       string
		source     string
		wantWarned bool
	}{
		{"v1 function entrypoint", "export const P = async () => ({})\n", true},
		{"v2 definition", "export default { id: \"p\", async setup() {} }\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			opencodev1.ResetWarned()
			root := t.TempDir()
			source := filepath.Join(root, openCodeSourcePath)
			require.NoError(t, os.MkdirAll(filepath.Dir(source), 0o755))
			require.NoError(t, os.WriteFile(source, []byte(tt.source), 0o644))
			m := &Manifest{Name: "test-plugin", Version: "1.2.3", SourceDir: root}

			// Act
			outputs, err := renderOpenCode(m, "/out")

			// Assert: generation still succeeds and copies the file verbatim.
			require.NoError(t, err)
			assert.Equal(t, tt.source, string(outputs[0].RawContent))
			assert.Equal(t, tt.wantWarned, opencodev1.WasWarned(source))
		})
	}
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
	assert.True(t, paths["/out/.opencode/ai-rulez-content.js"], "registration helper emitted")

	// A package-installed plugin is not a config directory, so OpenCode never
	// scans its .opencode/skills; the entrypoint has to register them itself.
	for _, o := range outputs {
		if filepath.ToSlash(o.Path) == "/out/.opencode/plugins/test-plugin.js" {
			assert.Contains(t, string(o.RawContent), `import { registerBundledContent } from "../ai-rulez-content.js"`)
			assert.Contains(t, string(o.RawContent), "await registerBundledContent(ctx)")
		}
	}
}

func TestOpenCodeContentHelperUsesV2Domains(t *testing.T) {
	helper := string(openCodeContentHelper)

	// Shapes verified against OpenCode 2.0.20: skills are added with their
	// content, commands are executors, agents are updated by id.
	for _, want := range []string{
		"ctx.skill.transform",
		"editor.add(skill)",
		"ctx.command.transform",
		"ctx.session.prompt(",
		"ctx.agent.transform",
		"editor.update(agent.id",
		"export async function registerBundledContent(ctx)",
	} {
		assert.Contains(t, helper, want)
	}
}

func TestOpenCodePackageNameFallsBackWithoutGitHubRepository(t *testing.T) {
	assert.Equal(t, "opencode-example", openCodePackageName(&Manifest{Name: "example"}))
}
