package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/govview"
)

// catalogConfigProject is the roles fixture with extra [catalog] settings.
func catalogConfigProject(t *testing.T, table string) {
	t.Helper()
	root := rolesCmdProject(t)
	path := filepath.Join(root, ".ai-rulez", "config.toml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	writeFile(t, path, string(data)+"\n[catalog]\n"+table)
}

func TestCatalogConfigTableSetsTheSiteDefaults(t *testing.T) {
	// Arrange
	catalogConfigProject(t, "title = \"Acme catalog\"\nmax_items_per_page = 2\nindexable = true\nexclude_owners = true\n")
	resetCatalogFlags(t)
	dir := filepath.Join(t.TempDir(), "site")
	catalogHTMLDir = dir

	// Act
	require.NoError(t, runCatalog(&bytes.Buffer{}))

	// Assert
	files := treeOf(t, dir)
	assert.Contains(t, files["index.html"], "<title>Overview - Acme catalog</title>")
	assert.Equal(t, 3, strings.Count(files["index.html"], "<tbody"), "5 items at 2 per page")
	assert.NotContains(t, files, "robots.txt", "indexable = true")
	var doc govview.CatalogDocV2
	require.NoError(t, json.Unmarshal([]byte(files["catalog.json"]), &doc))
	for _, it := range doc.Items {
		assert.Empty(t, it.Owner)
		assert.Nil(t, it.Excerpt, "an indexable site has no excerpts unless asked for")
	}
	assert.Contains(t, doc.Notes, "owners were switched off: item owners are omitted")
}

func TestCatalogFlagsOverrideTheCatalogTable(t *testing.T) {
	// Arrange
	catalogConfigProject(t, "title = \"From config\"\nmax_items_per_page = 2\nindexable = true\ninclude_excerpt = false\n")
	resetCatalogFlags(t)
	dir := filepath.Join(t.TempDir(), "site")
	catalogHTMLDir, catalogTitle = dir, "From flag"
	catalogPageSize, catalogPageSizeSet = 100, true
	catalogIndexable, catalogIndexableSet = false, true
	catalogExcerpt, catalogExcerptSet = true, true

	// Act
	require.NoError(t, runCatalog(&bytes.Buffer{}))

	// Assert
	files := treeOf(t, dir)
	assert.Contains(t, files["index.html"], "From flag")
	assert.Equal(t, 1, strings.Count(files["index.html"], "<tbody"))
	assert.Contains(t, files, "robots.txt", "--indexable=false wins over the table")
	assert.Contains(t, files["catalog.json"], `"excerpt"`)
}

func TestCatalogConfigIncludeExcerptAppliesToJSONToo(t *testing.T) {
	// Arrange
	catalogConfigProject(t, "include_excerpt = false\nexclude_owners = true\n")
	resetCatalogFlags(t)
	catalogFormat, catalogSchemaFlag = formatJSON, govview.CatalogSchemaVersionV2
	var out bytes.Buffer

	// Act
	require.NoError(t, runCatalog(&out))

	// Assert
	assert.NotContains(t, out.String(), `"excerpt"`)
	assert.NotContains(t, out.String(), `"owner"`)
	validateAgainst(t, "../../schema/catalog.schema.json", out.Bytes())
}

func TestCatalogNoOwnersFlag(t *testing.T) {
	// Arrange
	hostileCatalogProject(t)
	resetCatalogFlags(t)
	catalogFormat, catalogSchemaFlag, catalogNoOwners = formatJSON, govview.CatalogSchemaVersionV2, true
	var out bytes.Buffer

	// Act
	require.NoError(t, runCatalog(&out))

	// Assert
	assert.NotContains(t, out.String(), `"owner"`)
}

func TestCatalogPageFlagValidation(t *testing.T) {
	tests := []struct {
		name string
		set  func()
		want string
	}{
		{"page size without html", func() { catalogPageSize, catalogPageSizeSet = 5, true }, "apply to --html only"},
		{"negative page size", func() { catalogHTMLDir, catalogPageSize, catalogPageSizeSet = "x", -1, true }, "must not be negative"},
		{"no-owners for text", func() { catalogNoOwners = true }, "--no-owners applies to"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			resetCatalogFlags(t)
			tt.set()

			// Act
			err := checkCatalogFlags()

			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}
