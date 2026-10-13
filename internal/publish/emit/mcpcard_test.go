package emit

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/mcpcard"
)

func cardServers() []mcpcard.Server {
	return []mcpcard.Server{
		{
			Name: "docs", Description: "Search the docs.", Transport: "http", URL: "https://docs.acme.test/mcp",
			Headers: []mcpcard.Header{{Name: "Authorization", Secret: true}},
		},
		{Name: "local-tools", Description: "Local helper.", Transport: "stdio", Command: "npx", Args: []string{"-y", "tools"}},
		{Name: "feed", Description: "Release feed.", Transport: "sse", URL: "https://feed.acme.test/sse"},
	}
}

func mcpCardInput() Input { return fixtureFor(MCPServerCardEmitter) }

func TestMCPServerCard_OneFilePerRemoteServer(t *testing.T) {
	// Act
	files, findings, err := mcpServerCard{}.Emit(mcpCardInput())

	// Assert
	require.NoError(t, err)
	require.Len(t, files, 2)
	assert.Equal(t, "docs.json", files[0].Path)
	assert.Equal(t, "feed.json", files[1].Path)
	var card map[string]any
	require.NoError(t, json.Unmarshal(files[0].Data, &card))
	assert.Equal(t, mcpcard.SchemaURL, card["$schema"])
	assert.Equal(t, "io.github.acme/docs", card["name"])
	assert.Equal(t, "1.4.0", card["version"])
	assert.Equal(t, []any{map[string]any{"type": "streamable-http", "url": "https://docs.acme.test/mcp",
		"headers": []any{map[string]any{"name": "Authorization", "isRequired": true, "isSecret": true}}}}, card["remotes"])
	require.Len(t, findings, 1)
	assert.Contains(t, findings[0].Message, `"local-tools"`)
}

func TestMCPServerCard_NeverWritesAHeaderValue(t *testing.T) {
	files, _, err := mcpServerCard{}.Emit(mcpCardInput())

	require.NoError(t, err)
	for _, f := range files {
		assert.NotContains(t, string(f.Data), "secret-token", f.Path)
		assert.NotContains(t, string(f.Data), `"value"`, f.Path)
	}
}

func TestMCPServerCard_NamespaceSources(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Input)
		want   string
	}{
		{"from the repository", func(*Input) {}, "io.github.acme/docs"},
		{"from the ard publisher", func(in *Input) { in.ARD = &ARDInput{Publisher: "acme.test"} }, "test.acme/docs"},
		{"the option wins", func(in *Input) {
			in.ARD = &ARDInput{Publisher: "acme.test"}
			in.Options = map[string]string{"namespace": "com.example"}
		}, "com.example/docs"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := mcpCardInput()
			tt.mutate(&in)

			files, _, err := mcpServerCard{}.Emit(in)

			require.NoError(t, err)
			assert.Contains(t, string(files[0].Data), `"name": "`+tt.want+`"`)
		})
	}
}

func TestMCPServerCard_VersionOption(t *testing.T) {
	in := mcpCardInput()
	in.Options = map[string]string{"version": "2.0.0"}

	files, _, err := mcpServerCard{}.Emit(in)

	require.NoError(t, err)
	assert.Contains(t, string(files[0].Data), `"version": "2.0.0"`)
}

func TestMCPServerCard_Refusals(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Input)
		want   string
	}{
		{"no namespace", func(in *Input) { in.Repo = "" }, "namespace"},
		{"no remote server", func(in *Input) { in.Servers = in.Servers[1:2] }, "remote"},
		{"no server", func(in *Input) { in.Servers = nil }, "remote"},
		{"credentials in the url", func(in *Input) { in.Servers[0].URL = "https://u:p@docs.acme.test/mcp" }, "credentials"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := mcpCardInput()
			tt.mutate(&in)

			files, _, err := mcpServerCard{}.Emit(in)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
			assert.Empty(t, files)
		})
	}
}
