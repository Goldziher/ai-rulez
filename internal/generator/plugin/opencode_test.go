package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/opencodev1"
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

func bundleFrom(t *testing.T, m *Manifest) map[string]any {
	t.Helper()
	outputs, err := renderOpenCode(m, "/out")
	require.NoError(t, err)
	for _, o := range outputs {
		if filepath.ToSlash(o.Path) == "/out/.opencode/ai-rulez-bundle.json" {
			var doc map[string]any
			require.NoError(t, json.Unmarshal(o.RawContent, &doc))
			return doc
		}
	}
	return nil
}

func TestRenderOpenCodeBundlesMCPServers(t *testing.T) {
	m := &Manifest{
		Name:      "test-plugin",
		Version:   "1.2.3",
		SourceDir: t.TempDir(),
		MCP: []config.PluginMCPLaunch{
			{
				Name:      "local",
				Command:   "${PLUGIN_ROOT}/scripts/launch.sh",
				Args:      []string{"--port", "${PORT}"},
				Env:       map[string]string{"TOKEN": "${API_TOKEN}"},
				Transport: config.TransportStdio,
			},
			{Name: "remote", Transport: config.TransportHTTP, URL: "https://mcp.example.com"},
		},
	}

	bundle := bundleFrom(t, m)

	require.NotNil(t, bundle, "MCP servers alone are enough to emit the bundle")
	servers := bundle["mcp"].(map[string]any)
	local := servers["local"].(map[string]any)
	assert.Equal(t, "local", local["type"])
	assert.Equal(t, []any{"${PLUGIN_ROOT}/scripts/launch.sh", "--port", "${PORT}"}, local["command"])
	assert.Equal(t, map[string]any{"TOKEN": "${API_TOKEN}"}, local["environment"],
		"env references stay as references; the helper resolves them from process.env at runtime")
	remote := servers["remote"].(map[string]any)
	assert.Equal(t, "remote", remote["type"])
	assert.Equal(t, "https://mcp.example.com", remote["url"])
	assert.NotContains(t, remote, "command")
}

func TestRenderOpenCodeEntrypointRegistersWhenOnlyMCP(t *testing.T) {
	m := &Manifest{
		Name:      "test-plugin",
		Version:   "1.2.3",
		SourceDir: t.TempDir(),
		MCP:       []config.PluginMCPLaunch{{Name: "s", Command: "x", Transport: config.TransportStdio}},
	}

	outputs, err := renderOpenCode(m, "/out")
	require.NoError(t, err)

	assert.Contains(t, string(outputs[0].RawContent), "await registerBundledContent(ctx)")
}

func TestRenderOpenCodeAgentSettings(t *testing.T) {
	tests := []struct {
		name  string
		extra map[string]string
		cfg   *config.Config
		want  map[string]any
		omit  []string
	}{
		{
			name:  "provider-qualified model with variant",
			extra: map[string]string{"model": "anthropic/claude-sonnet-4#high"},
			want:  map[string]any{"model": "anthropic/claude-sonnet-4", "variant": "high", "mode": "all"},
		},
		{
			name:  "bare alias is omitted",
			extra: map[string]string{"model": "sonnet"},
			want:  map[string]any{"mode": "all"},
			omit:  []string{"model"},
		},
		{
			name:  "opencode_model override wins over a bare alias",
			extra: map[string]string{"model": "sonnet", "opencode_model": "openai/gpt-5"},
			want:  map[string]any{"model": "openai/gpt-5"},
		},
		{
			name:  "per-preset default model",
			extra: map[string]string{},
			cfg:   &config.Config{Defaults: &config.DefaultsConfig{ModelByPreset: map[string]string{"opencode": "openai/gpt-5"}}},
			want:  map[string]any{"model": "openai/gpt-5"},
		},
		{
			name:  "sampling, hidden, mode and description",
			extra: map[string]string{"temperature": "0.2", "top_p": "0.9", "hidden": "true", "mode": "subagent", "description": "Reviews"},
			want: map[string]any{
				"temperature": 0.2, "top_p": 0.9, "hidden": true, "mode": "subagent", "description": "Reviews",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := tt.cfg
			if cfg == nil {
				cfg = &config.Config{}
			}
			m := &Manifest{
				Name:      "test-plugin",
				Version:   "1.2.3",
				SourceDir: t.TempDir(),
				Config:    cfg,
				Agents:    []config.ContentFile{{Name: "rev", Path: "builtin://rev", Metadata: &config.Metadata{Extra: tt.extra}}},
			}

			// Act
			bundle := bundleFrom(t, m)

			// Assert
			require.NotNil(t, bundle)
			agent := bundle["agents"].(map[string]any)["rev"].(map[string]any)
			for key, want := range tt.want {
				assert.Equal(t, want, agent[key], key)
			}
			for _, key := range tt.omit {
				assert.NotContains(t, agent, key)
			}
		})
	}
}

func TestOpenCodeContentHelperRegistersMCPAndAgentSettings(t *testing.T) {
	helper := string(openCodeContentHelper)

	for _, want := range []string{
		"ai-rulez-bundle.json",
		"ctx.mcp.transform",
		"editor.set(",
		"process.env",
		"${PLUGIN_ROOT}",
		"request.body",
		"providerID",
	} {
		assert.Contains(t, helper, want)
	}
}

func TestOpenCodePublishedFilesIncludeReferencedPaths(t *testing.T) {
	tests := []struct {
		name  string
		mcp   []config.PluginMCPLaunch
		tree  map[string]string
		want  []string
		extra string
	}{
		{
			name: "directory referenced by command",
			mcp:  []config.PluginMCPLaunch{{Name: "s", Command: "${PLUGIN_ROOT}/scripts/run.sh", Transport: config.TransportStdio}},
			tree: map[string]string{"scripts/run.sh": "#!/bin/sh\n"},
			want: []string{".opencode/", "assets/", "README.md", "scripts/"},
		},
		{
			name: "file at the top level, args and env, deduplicated",
			mcp: []config.PluginMCPLaunch{{
				Name: "s", Command: "node", Args: []string{"${PLUGIN_ROOT}/server.js", "${PLUGIN_ROOT}/scripts/a"},
				Env: map[string]string{"CFG": "${PLUGIN_ROOT}/scripts/b"}, Transport: config.TransportStdio,
			}},
			tree: map[string]string{"server.js": "x", "scripts/a": "x", "scripts/b": "x"},
			want: []string{".opencode/", "assets/", "README.md", "scripts/", "server.js"},
		},
		{
			name: "missing path is not listed",
			mcp:  []config.PluginMCPLaunch{{Name: "s", Command: "${PLUGIN_ROOT}/bin/run", Transport: config.TransportStdio}},
			tree: map[string]string{},
			want: []string{".opencode/", "assets/", "README.md"},
		},
		{
			name: "traversal and already-listed paths are ignored",
			mcp: []config.PluginMCPLaunch{{
				Name: "s", Command: "${PLUGIN_ROOT}/../etc/x", Args: []string{"${PLUGIN_ROOT}/assets/y"}, Transport: config.TransportStdio,
			}},
			tree: map[string]string{"assets/y": "x"},
			want: []string{".opencode/", "assets/", "README.md"},
		},
		{
			name: "remote url is scanned too",
			mcp:  []config.PluginMCPLaunch{{Name: "s", Transport: config.TransportHTTP, URL: "file://${PLUGIN_ROOT}/sock/s"}},
			tree: map[string]string{"sock/s": "x"},
			want: []string{".opencode/", "assets/", "README.md", "sock/"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			for rel, content := range tt.tree {
				require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(root, rel)), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(root, rel), []byte(content), 0o644))
			}
			m := &Manifest{Name: "p", Version: "1.0.0", SourceDir: root, MCP: tt.mcp}

			// Act
			outputs, err := renderOpenCode(m, "/out")
			require.NoError(t, err)

			// Assert
			var pkg struct {
				Files []string `json:"files"`
			}
			require.NoError(t, json.Unmarshal(outputs[1].RawContent, &pkg))
			assert.Equal(t, tt.want, pkg.Files)
		})
	}
}
