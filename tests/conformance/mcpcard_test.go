package conformance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaptinlin/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/mcpcard"
)

const (
	// mcpCardSchemaID names the vendored schema.json, which declares only $defs.
	mcpCardSchemaID = "https://raw.githubusercontent.com/modelcontextprotocol/experimental-ext-server-card/" + mcpcard.SpecCommit + "/schema.json"
	mcpCardRootID   = "urn:conformance:mcp-server-card"
)

// The MCP Server Card (SEP-2127, Final) has no published, versioned schema yet:
// schema.json of the experimental server-card extension is generated from
// schema.ts and pinned at mcpcard.SpecCommit. It declares only $defs, so the
// root is a reference to ServerCard.
func mcpCardSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	docs := map[string][]byte{
		mcpCardSchemaID: schemaFile(t, "mcp-server-card/server-card.schema.json"),
		mcpCardRootID:   []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","$ref":"` + mcpCardSchemaID + `#/$defs/ServerCard"}`),
	}
	return compileDocs(t, mcpCardRootID, docs)
}

func TestMCPServerCardGoldensConformToTheExtensionSchema(t *testing.T) {
	// Arrange
	schema := mcpCardSchema(t)
	cards, err := filepath.Glob(filepath.Join(repoRoot, "internal", "publish", "emit", "testdata", "golden", "mcp-server-card", "*.json"))
	require.NoError(t, err)
	require.NotEmpty(t, cards)

	// Assert: every standalone card, then every card embedded in the ARD golden
	for _, path := range cards {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path) //nolint:gosec // a checked-in golden
			require.NoError(t, err)
			requireValid(t, schema, data)
		})
	}
	var ard struct {
		Entries []struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		} `json:"entries"`
	}
	require.NoError(t, json.Unmarshal(repoFile(t, "internal/publish/emit/testdata/golden/ard/ard.json"), &ard))
	embedded := 0
	for _, e := range ard.Entries {
		if e.Type == mcpcard.MediaType {
			requireValid(t, schema, e.Data)
			embedded++
		}
	}
	assert.Positive(t, embedded, "the ARD golden embeds a card")
}

func TestMCPServerCardBuilderOutputConformsToTheExtensionSchema(t *testing.T) {
	tests := []struct {
		name   string
		server mcpcard.Server
	}{
		{"streamable http", mcpcard.Server{Name: "docs", Description: "Search the docs.", Transport: "http", URL: "https://docs.acme.test/mcp"}},
		{"sse", mcpcard.Server{Name: "feed", Description: "Feed.", Transport: "sse", URL: "https://feed.acme.test/sse"}},
		{"a url alone", mcpcard.Server{Name: "api", URL: "http://localhost:8080/mcp"}},
		{"a long description and an unusual name", mcpcard.Server{
			Name: "My Server v2", Description: strings.Repeat("long ", 40),
			Transport: "http", URL: "https://a.test/mcp",
		}},
		{"headers", mcpcard.Server{
			Name: "authed", Description: "Needs a token.", Transport: "http", URL: "https://a.test/mcp",
			Headers: []mcpcard.Header{{Name: "Authorization", Secret: true}, {Name: "X-Region"}},
		}},
	}
	schema := mcpCardSchema(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			cards, _, err := mcpcard.Build([]mcpcard.Server{tt.server}, mcpcard.Options{Namespace: "test.acme", Version: "1.4.0"})
			require.NoError(t, err)
			require.Len(t, cards, 1)
			data, err := cards[0].JSON()
			require.NoError(t, err)

			// Assert
			requireValid(t, schema, data)
		})
	}
}

func TestMCPServerCardSchemaRejectsWhatACardMustNotBe(t *testing.T) {
	schema := mcpCardSchema(t)
	valid := `"$schema":"` + mcpcard.SchemaURL + `","name":"test.acme/docs","description":"Docs.","version":"1.0.0"`
	requireValid(t, schema, []byte(`{`+valid+`}`))
	requireInvalid(t, schema, `{"name":"test.acme/docs","description":"Docs.","version":"1.0.0"}`)
	requireInvalid(t, schema, `{"$schema":"`+mcpcard.SchemaURL+`","name":"docs","description":"Docs.","version":"1.0.0"}`)
	requireInvalid(t, schema, `{"$schema":"https://example.com/other.json","name":"test.acme/docs","description":"Docs.","version":"1.0.0"}`)
	requireInvalid(t, schema, `{`+valid+`,"remotes":[{"type":"stdio","url":"https://a.test/mcp"}]}`)
	requireInvalid(t, schema, `{`+valid+`,"remotes":[{"type":"sse","url":"ftp://a.test/mcp"}]}`)
	requireInvalid(t, schema, `{`+valid+`,"remotes":[{"type":"sse"}]}`)
}
