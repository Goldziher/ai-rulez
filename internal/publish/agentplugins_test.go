package publish

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/agentplugins"
)

func agentPluginsInput(t *testing.T, p *agentplugins.Plugin, opts agentplugins.Options) Input {
	t.Helper()
	built, _, err := agentplugins.Build(p, opts)
	require.NoError(t, err)
	in := sampleInput()
	in.Runtimes = []string{"agent-plugins"}
	in.Files = nil
	for path, data := range built {
		in.Files = append(in.Files, File{Path: path, Data: data})
	}
	return in
}

func validAgentPlugin() *agentplugins.Plugin {
	return &agentplugins.Plugin{
		Metadata:   agentplugins.Metadata{Name: "acme", Version: "1.4.0", Description: "Acme"},
		Skills:     []agentplugins.Skill{{Name: "deploy", SkillMD: []byte("---\nname: deploy\ndescription: Deploy.\n---\n")}},
		MCPServers: []agentplugins.MCPServer{{Name: "api", Transport: "http", URL: "https://api.acme.test/mcp"}},
	}
}

func TestBuild_AcceptsAValidAgentPluginsPackage(t *testing.T) {
	_, err := Build(agentPluginsInput(t, validAgentPlugin(), agentplugins.Options{Spec: "1.1.0"}))
	require.NoError(t, err)
}

func TestBuild_RejectsAnAgentPluginsPackageThatBreaksTheSpec(t *testing.T) {
	cases := map[string]func(in *Input){
		"skill without description": func(in *Input) {
			setFile(in, "skills/deploy/SKILL.md", "---\nname: deploy\n---\n")
		},
		"placeholder in mcp args": func(in *Input) {
			setFile(in, "mcp.json", `{"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json","mcpServers":{"s":{"type":"stdio","command":"srv","args":["${API_KEY}"]}}}`)
		},
		"plain http server": func(in *Input) {
			setFile(in, "mcp.json", `{"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json","mcpServers":{"s":{"type":"streamable-http","url":"http://example.com/mcp"}}}`)
		},
	}
	wantCode := map[string]string{
		"skill without description": agentplugins.RuleSkill,
		"placeholder in mcp args":   agentplugins.RulePlaceholder,
		"plain http server":         agentplugins.RuleMCP,
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			in := agentPluginsInput(t, validAgentPlugin(), agentplugins.Options{})
			mutate(&in)

			_, err := Build(in)

			var pe *Error
			require.ErrorAs(t, err, &pe)
			assert.Equal(t, wantCode[name], pe.Code)
			assert.Equal(t, ExitGate, pe.Exit)
		})
	}
}

func TestBuild_IgnoresBundlesWithoutARootPluginJSON(t *testing.T) {
	_, err := Build(sampleInput())
	require.NoError(t, err)
}

func setFile(in *Input, path, body string) {
	for i := range in.Files {
		if in.Files[i].Path == path {
			in.Files[i].Data = []byte(body)
			return
		}
	}
	in.Files = append(in.Files, File{Path: path, Data: []byte(body)})
}
