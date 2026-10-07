package emit

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/agentplugins"
)

// agentPluginFixtureFiles are the files the agent-plugins runtime writes for
// the shared fixture plugin, built through the library.
func agentPluginFixtureFiles() []File {
	files, _, err := agentplugins.Build(&agentplugins.Plugin{
		Metadata: agentplugins.Metadata{Name: "acme-conventions", Version: "1.4.0", Description: "Conventions", Keywords: []string{"conventions"}},
		Skills:   []agentplugins.Skill{{Name: "deploy", SkillMD: []byte(skillMD)}},
		MCPServers: []agentplugins.MCPServer{
			{Name: "docs", Transport: "http", URL: "https://docs.acme.test/mcp"},
			{Name: "local", Command: "${PLUGIN_ROOT}/bin/server", Args: []string{"serve"}},
		},
		Files: map[string][]byte{"bin/server": []byte("#!/bin/sh\n")},
	}, agentplugins.Options{})
	if err != nil {
		panic(err)
	}
	var out []File
	for p, data := range files {
		out = append(out, File{Path: p, Data: data})
	}
	return out
}

func TestAgentPlugins_WritesOnlyThePortablePackage(t *testing.T) {
	files, _, err := agentPlugins{}.Emit(fixtureFor("agent-plugins"))
	require.NoError(t, err)

	var paths []string
	mem := fstest.MapFS{}
	for _, f := range files {
		paths = append(paths, f.Path)
		mem[f.Path[len("acme-conventions/"):]] = &fstest.MapFile{Data: f.Data}
	}
	assert.Equal(t, []string{
		"acme-conventions/bin/server", "acme-conventions/mcp.json", "acme-conventions/plugin.json",
		"acme-conventions/skills/deploy/SKILL.md",
	}, paths, "the claude and cursor manifests of the bundle are not part of the package")
	res := agentplugins.Validate(mem)
	assert.False(t, res.Rejected)
	assert.False(t, agentplugins.HasErrors(res.Findings), "%v", res.Findings)
}

func TestAgentPlugins_SpecOptionRewritesTheSchemaIdentifiers(t *testing.T) {
	in := fixtureFor("agent-plugins")
	in.Options = map[string]string{"spec": "1.1.0"}

	files, _, err := agentPlugins{}.Emit(in)

	require.NoError(t, err)
	for _, f := range files {
		switch f.Path {
		case "acme-conventions/plugin.json":
			assert.Contains(t, string(f.Data), agentplugins.PluginSchemaID("1.1.0"))
		case "acme-conventions/mcp.json":
			assert.Contains(t, string(f.Data), agentplugins.MCPSchemaID("1.1.0"))
		}
	}
}

func TestAgentPlugins_RefusesWhatItCannotPackage(t *testing.T) {
	t.Run("no package in the bundle", func(t *testing.T) {
		_, _, err := agentPlugins{}.Emit(fixture())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "agent-plugins")
	})
	t.Run("unknown spec", func(t *testing.T) {
		in := fixtureFor("agent-plugins")
		in.Options = map[string]string{"spec": "9.9.9"}
		_, _, err := agentPlugins{}.Emit(in)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "9.9.9")
	})
	t.Run("rejected plugin.json", func(t *testing.T) {
		in := fixtureFor("agent-plugins")
		for i := range in.Plugins[0].Files {
			if in.Plugins[0].Files[i].Path == "plugin.json" {
				in.Plugins[0].Files[i].Data = []byte("{\"name\": 1}")
			}
		}
		_, _, err := agentPlugins{}.Emit(in)
		require.Error(t, err)
	})
}

func TestAgentPluginEntries(t *testing.T) {
	assert.Empty(t, AgentPluginEntries(fixture()))
	got := AgentPluginEntries(fixtureFor("agent-plugins"))
	assert.Equal(t, []AgentPluginEntry{{Name: "acme-conventions", Version: "1.4.0", Path: "acme-conventions"}}, got)
}

func TestAgentPlugins_KeepsTheSpecOfTheBundle(t *testing.T) {
	in := fixtureFor("agent-plugins")
	files, _, err := agentplugins.Build(&agentplugins.Plugin{Metadata: agentplugins.Metadata{Name: "acme-conventions", Version: "1.4.0"}},
		agentplugins.Options{Spec: "1.1.0"})
	require.NoError(t, err)
	in.Plugins[0].Files = []File{{Path: "plugin.json", Data: files["plugin.json"]}}

	out, _, err := agentPlugins{}.Emit(in)

	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Contains(t, string(out[0].Data), agentplugins.PluginSchemaID("1.1.0"))
}
