package importer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/agentplugins"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
)

const apSkill = "---\nname: deploy\ndescription: Deploy the service safely.\n---\n\n# Deploy\n\nRun `scripts/run.sh`.\n"

// agentPluginTree is a package built by the library, as files.
func agentPluginTree(t *testing.T, p *agentplugins.Plugin, spec string) map[string]string {
	t.Helper()
	built, _, err := agentplugins.Build(p, agentplugins.Options{Spec: spec})
	require.NoError(t, err)
	out := map[string]string{}
	for path, data := range built {
		out[path] = string(data)
	}
	return out
}

func fullPlugin() *agentplugins.Plugin {
	return &agentplugins.Plugin{
		Metadata: agentplugins.Metadata{
			Name: "acme.tools", Version: "1.2.0", Description: "Acme tooling.", License: "MIT",
			Homepage: "https://acme.test", Repository: "https://github.com/acme/tools", Keywords: []string{"a", "b"},
			Author: &agentplugins.Author{Name: "Acme", Email: "dev@acme.test"},
		},
		Skills: []agentplugins.Skill{{
			Name: "deploy", SkillMD: []byte(apSkill),
			Files: map[string][]byte{"scripts/run.sh": []byte("#!/bin/sh\necho run\n"), "references/notes.md": []byte("notes\n")},
		}},
		MCPServers: []agentplugins.MCPServer{
			{Name: "local", Command: "${PLUGIN_ROOT}/bin/server", Args: []string{"--data", "${PLUGIN_DATA}/x"}, Env: map[string]string{"MODE": "fast"}},
			{Name: "docs", Transport: "http", URL: "https://docs.acme.test/mcp", Headers: map[string]string{"X-Team": "acme"}},
			{Name: "events", Transport: "sse", URL: "https://events.acme.test/sse"},
		},
		Extensions: []agentplugins.Extension{
			{Namespace: agentplugins.NamespaceClaudeCode, Files: map[string][]byte{
				"agents/reviewer.md": []byte("---\nname: reviewer\ndescription: Reviews.\n---\nReview.\n"),
				"commands/ship.md":   []byte("---\ndescription: Ship it.\n---\nShip.\n"),
				"hooks/hooks.json":   []byte("{}\n"),
			}},
			{Namespace: agentplugins.NamespaceAIRulez, Files: map[string][]byte{"rules/style.md": []byte("# Style\n\nBe terse.\n")}},
			{Namespace: "com.example.client", Manifest: map[string]any{"x": true}},
		},
		Files: map[string][]byte{"LICENSE": []byte("MIT\n")},
	}
}

func TestAgentPluginsDetect(t *testing.T) {
	schema := agentplugins.PluginSchemaID("1.0.0")
	tests := []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{"agent plugins manifest", map[string]string{"plugin.json": `{"$schema":"` + schema + `","name":"a"}`}, []string{"plugin.json"}},
		{"plugin.json without the schema", map[string]string{"plugin.json": `{"name":"a"}`}, nil},
		{"another schema", map[string]string{"plugin.json": `{"$schema":"https://example.test/s.json"}`}, nil},
		{"generated into an ai-rulez project", map[string]string{
			"plugin.json": `{"$schema":"` + schema + `","name":"a"}`, ".ai-rulez/config.toml": "version = \"5.0\"\n",
		}, nil},
		{"not json", map[string]string{"plugin.json": "nope"}, nil},
		{"nothing", map[string]string{"README.md": "x"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, agentPluginsImporter{}.Detect(mapFS(tt.files)))
		})
	}
}

func TestAgentPluginsPlan_MapsEveryComponent(t *testing.T) {
	plan, err := agentPluginsImporter{}.Plan(mapFS(agentPluginTree(t, fullPlugin(), "1.1.0")), Options{})
	require.NoError(t, err)

	require.NotNil(t, plan.Plugin)
	assert.Equal(t, "acme.tools", plan.Plugin.Name)
	assert.Equal(t, "1.2.0", plan.Plugin.Version)
	assert.Equal(t, "1.1.0", plan.Plugin.Spec)
	assert.Equal(t, []string{"agent-plugins"}, plan.Plugin.Runtimes)
	assert.Equal(t, "dev@acme.test", plan.Plugin.Author.Email)

	byName := map[string]Item{}
	for _, it := range plan.Items {
		byName[string(it.Kind)+"/"+it.Name] = it
	}
	require.Contains(t, byName, "skill/deploy")
	assert.Equal(t, apSkill, string(byName["skill/deploy"].Main))
	assert.Len(t, byName["skill/deploy"].Resources, 2)
	assert.Contains(t, byName, "agent/reviewer")
	assert.Contains(t, byName, "command/ship")
	assert.Contains(t, byName, "rule/style")

	servers := map[string]config.MCPServer{}
	for _, s := range plan.MCPServers {
		servers[s.Name] = s
	}
	assert.Equal(t, "./bin/server", servers["local"].Command)
	assert.Equal(t, config.TransportHTTP, servers["docs"].Transport)
	assert.Equal(t, map[string]string{"X-Team": "acme"}, servers["docs"].Headers)
	assert.Equal(t, config.TransportSSE, servers["events"].Transport)
}

func TestAgentPluginsPlan_ReportsWhatItCannotMap(t *testing.T) {
	plan, err := agentPluginsImporter{}.Plan(mapFS(agentPluginTree(t, fullPlugin(), "1.0.0")), Options{})
	require.NoError(t, err)

	dropped := map[string]bool{}
	for _, f := range plan.Findings {
		if f.Status == StatusDropped {
			dropped[f.Source] = true
		}
	}
	assert.True(t, dropped["LICENSE"], "a bundled root file has no source")
	assert.True(t, dropped["com.anthropic.claude-code/hooks/hooks.json"])
	assert.True(t, dropped["com.example.client"], "an unknown namespace")
}

func TestAgentPluginsPlan_WithoutVersionOrDescriptionWritesNoPluginBlock(t *testing.T) {
	files := agentPluginTree(t, &agentplugins.Plugin{Metadata: agentplugins.Metadata{Name: "bare"}}, "1.0.0")

	plan, err := agentPluginsImporter{}.Plan(mapFS(files), Options{})

	require.NoError(t, err)
	assert.Nil(t, plan.Plugin)
	var action bool
	for _, f := range plan.Findings {
		action = action || f.Status == StatusNeedsAction
	}
	assert.True(t, action)
}

func TestAgentPluginsPlan_RejectsABrokenManifest(t *testing.T) {
	_, err := agentPluginsImporter{}.Plan(mapFS(map[string]string{"plugin.json": `{"$schema":"` + agentplugins.PluginSchemaID("1.0.0") + `","name":5}`}), Options{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), CodeInvalid)
}

func TestConvert_AgentPluginsIsDetectedByAuto(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, agentPluginTree(t, fullPlugin(), "1.0.0"))

	report, err := Convert(context.Background(), ConvertOptions{Source: dir, Write: true})

	require.NoError(t, err)
	require.True(t, report.Written, "%+v", report)
	cfg, err := os.ReadFile(filepath.Join(dir, ".ai-rulez", "config.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(cfg), "[plugin]")
	assert.Contains(t, string(cfg), "name = 'acme.tools'")
	assert.FileExists(t, filepath.Join(dir, ".ai-rulez", "skills", "deploy", "scripts", "run.sh"))
}

// exportPlugin generates the plugin bundle of the project at root and returns
// its files by path.
func exportPlugin(t *testing.T, root string) map[string]string {
	t.Helper()
	cfg, err := config.LoadConfig(context.Background(), root, config.WithoutLocal())
	require.NoError(t, err)
	files, err := generator.NewGenerator(cfg).PluginFiles("")
	require.NoError(t, err)
	out := map[string]string{}
	for _, f := range files {
		out[f.Path] = string(f.Data)
	}
	return out
}

func roundTripProject(t *testing.T, spec string) string {
	t.Helper()
	root := t.TempDir()
	cfg := `version = "5.0"
name = "acme"
presets = ["claude"]

[plugin]
name = "acme.tools"
description = "Acme tooling."
version = "1.2.0"
license = "MIT"
homepage = "https://acme.test"
keywords = ["a", "b"]
runtimes = ["agent-plugins"]
` + spec + `
[plugin.author]
name = "Acme"
email = "dev@acme.test"

[[mcp_servers]]
name = "local"
command = "${PLUGIN_ROOT}/bin/server"
args = ["--data", "${PLUGIN_DATA}/x"]

[mcp_servers.env]
MODE = "fast"

[[mcp_servers]]
name = "docs"
transport = "http"
url = "https://docs.acme.test/mcp"
`
	writeTree(t, root, map[string]string{
		".ai-rulez/config.toml":                       cfg,
		".ai-rulez/skills/deploy/SKILL.md":            apSkill,
		".ai-rulez/skills/deploy/scripts/run.sh":      "#!/bin/sh\necho run\n",
		".ai-rulez/skills/deploy/scripts/helper.py":   "print('hi')\n",
		".ai-rulez/skills/deploy/references/notes.md": "notes\n",
	})
	return root
}

// TestRoundTrip_ExportImportExportIsByteIdentical exports a project as an Agent
// Plugins package, imports the package into a fresh project and exports that
// again: the two packages, files and provenance sidecar included, are identical.
func TestRoundTrip_ExportImportExportIsByteIdentical(t *testing.T) {
	for _, spec := range []string{"", `spec = "1.1.0"`} {
		t.Run("spec "+spec, func(t *testing.T) {
			first := exportPlugin(t, roundTripProject(t, spec))
			require.Contains(t, first, "plugin.json")
			pkg := t.TempDir()
			writeTree(t, pkg, first)
			again := t.TempDir()

			report, err := Convert(context.Background(), ConvertOptions{
				Source: pkg, Into: filepath.Join(again, ".ai-rulez"), From: []string{"agent-plugins"}, Write: true,
			})
			require.NoError(t, err)
			require.True(t, report.Written, "%+v", report)

			assert.Equal(t, first, exportPlugin(t, again))
		})
	}
}
