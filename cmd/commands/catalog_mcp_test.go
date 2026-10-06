package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/govview"
)

const catalogMCPConfig = `
[[mcp_servers]]
name = "github"
command = "npx"
args = ["-y", "@modelcontextprotocol/server-github", "--api-key", "arg-secret-9"]
[mcp_servers.env]
GITHUB_TOKEN = "${GITHUB_TOKEN}"
LITERAL_TOKEN = "env-secret-7"

[[mcp_servers]]
name = "remote"
transport = "http"
url = "https://user:url-secret-3@internal.example/mcp"
[mcp_servers.headers]
Authorization = "Bearer header-secret-5"
`

// catalogMCPProject is the roles fixture plus two MCP servers that carry secrets.
func catalogMCPProject(t *testing.T) string {
	t.Helper()
	root := rolesCmdProject(t)
	path := filepath.Join(root, ".ai-rulez", "config.toml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	writeFile(t, path, string(data)+catalogMCPConfig)
	return root
}

var catalogMCPSecrets = []string{"arg-secret-9", "env-secret-7", "url-secret-3", "internal.example", "header-secret-5"}

func TestCatalogV2ListsMCPServersWithoutSecrets(t *testing.T) {
	// Arrange
	catalogMCPProject(t)
	resetCatalogFlags(t)
	catalogFormat, catalogSchemaFlag = formatJSON, govview.CatalogSchemaVersionV2
	var out bytes.Buffer

	// Act
	require.NoError(t, runCatalog(&out))

	// Assert
	validateAgainst(t, "../../schema/catalog.schema.json", out.Bytes())
	var doc govview.CatalogDocV2
	require.NoError(t, json.Unmarshal(out.Bytes(), &doc))
	require.Len(t, doc.MCPServers, 2)
	assert.Equal(t, "github", doc.MCPServers[0].Name)
	assert.Equal(t, "npx", doc.MCPServers[0].CommandBasename)
	assert.Equal(t, "remote", doc.MCPServers[1].Name)
	for _, secret := range catalogMCPSecrets {
		assert.NotContains(t, out.String(), secret)
	}
}

func TestCatalogHTMLWritesTheMCPPageWithoutSecrets(t *testing.T) {
	// Arrange
	catalogMCPProject(t)
	resetCatalogFlags(t)
	dir := filepath.Join(t.TempDir(), "site")
	catalogHTMLDir = dir
	var out bytes.Buffer

	// Act
	require.NoError(t, runCatalog(&out))

	// Assert
	files := treeOf(t, dir)
	assert.Contains(t, files["mcp.html"], "GITHUB_TOKEN (from ${GITHUB_TOKEN})")
	assert.Contains(t, files["mcp.html"], "LITERAL_TOKEN (literal value)")
	assert.Contains(t, out.String(), "2 MCP servers")
	for name, page := range files {
		for _, secret := range catalogMCPSecrets {
			assert.NotContainsf(t, page, secret, "%s", name)
		}
	}
}
