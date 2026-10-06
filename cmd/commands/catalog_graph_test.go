package commands

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/govview"
)

// catalogGraphProject adds an agent and a skill that use other skills to the
// roles fixture.
func catalogGraphProject(t *testing.T) {
	t.Helper()
	root := rolesCmdProject(t)
	writeFile(t, filepath.Join(root, ".ai-rulez", "agents", "reviewer.md"),
		"---\nname: reviewer\ndescription: Reviews changes\nskills:\n  - migrate\n  - ui\n  - ghost\n---\nReview.\n")
	writeFile(t, filepath.Join(root, ".ai-rulez", "domains", "backend", "skills", "migrate", "SKILL.md"),
		"---\nname: migrate\ndescription: Use when you need migrate.\nskills:\n  - deploy\n---\nBody of migrate.\n")
}

func TestCatalogV2CarriesDependencyEdges(t *testing.T) {
	// Arrange
	catalogGraphProject(t)
	resetCatalogFlags(t)
	catalogFormat, catalogSchemaFlag = formatJSON, govview.CatalogSchemaVersionV2
	var out bytes.Buffer

	// Act
	require.NoError(t, runCatalog(&out))

	// Assert
	validateAgainst(t, "../../schema/catalog.schema.json", out.Bytes())
	var doc govview.CatalogDocV2
	require.NoError(t, json.Unmarshal(out.Bytes(), &doc))
	assert.Contains(t, doc.Edges, govview.CatalogEdge{From: "agent/-/reviewer", To: "skill/backend/migrate", Kind: "uses"})
	assert.Contains(t, doc.Edges, govview.CatalogEdge{From: "agent/-/reviewer", To: "skill/frontend/ui", Kind: "uses"})
	assert.Contains(t, doc.Notes, "agent/-/reviewer names skill ghost, which is not in the catalog: the dependency is not drawn")
}

func TestCatalogHTMLWritesTheGraphPage(t *testing.T) {
	// Arrange
	catalogGraphProject(t)
	resetCatalogFlags(t)
	dir := filepath.Join(t.TempDir(), "site")
	catalogHTMLDir = dir

	// Act
	require.NoError(t, runCatalog(&bytes.Buffer{}))

	// Assert
	files := treeOf(t, dir)
	graph := files["graph.html"]
	assert.Contains(t, graph, "<svg")
	assert.Contains(t, graph, "agent/-/reviewer")
	assert.Contains(t, graph, `href="items/agent/-/reviewer.html"`)
	assert.Contains(t, files["index.html"], `href="graph.html"`)
}

func TestCatalogHTMLRoleScopeDropsEdgesOutsideTheRole(t *testing.T) {
	// Arrange
	catalogGraphProject(t)
	resetCatalogFlags(t)
	dir := filepath.Join(t.TempDir(), "site")
	catalogHTMLDir, catalogRole = dir, "base"

	// Act
	require.NoError(t, runCatalog(&bytes.Buffer{}))

	// Assert
	var doc govview.CatalogDocV2
	require.NoError(t, json.Unmarshal([]byte(treeOf(t, dir)["catalog.json"]), &doc))
	kept := map[string]bool{}
	for _, it := range doc.Items {
		kept[it.Ref] = true
	}
	for _, e := range doc.Edges {
		assert.True(t, kept[e.From] && kept[e.To], "edge %s -> %s leaves the role's items", e.From, e.To)
	}
}
