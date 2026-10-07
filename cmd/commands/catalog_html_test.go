package commands

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/catalogsite"
	"github.com/Goldziher/ai-rulez/v5/internal/govview"
)

var quotedValue = regexp.MustCompile(`="[^"]*"`)

func resetCatalogFlags(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		catalogFormat, catalogSchemaFlag, catalogHTMLDir, catalogRole = "", govview.CatalogSchemaVersion, "", ""
		catalogExcerpt, catalogExcerptSet, catalogIndexable, catalogClean = true, false, false, false
		catalogTitle, catalogAllowFindings = "", nil
		catalogWithEval, catalogWithUsage, catalogCheck = "", "", false
		catalogIndexableSet, catalogPageSizeSet, catalogNoOwners, catalogPageSize = false, false, false, 0
		catalogMarkdown, catalogMarkdownSet = false, false
	})
	catalogExcerpt = true
}

// treeOf reads every file below dir into a map keyed by slash path.
func treeOf(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	require.NoError(t, filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, readErr := os.ReadFile(p)
		require.NoError(t, readErr)
		rel, _ := filepath.Rel(dir, p) //nolint:errcheck // under dir
		out[filepath.ToSlash(rel)] = string(data)
		return nil
	}))
	return out
}

func TestCatalogJSONSchemaVersions(t *testing.T) {
	tests := []struct {
		name        string
		schemaFlag  int
		wantVersion int
		schema      string
	}{
		{"default is version 1", govview.CatalogSchemaVersion, 1, "../../schema/catalog.v1.schema.json"},
		{"version 2 on request", govview.CatalogSchemaVersionV2, 2, "../../schema/catalog.schema.json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			rolesCmdProject(t)
			resetCatalogFlags(t)
			catalogFormat, catalogSchemaFlag = formatJSON, tt.schemaFlag
			var out bytes.Buffer

			// Act
			err := runCatalog(&out)

			// Assert
			require.NoError(t, err)
			validateAgainst(t, tt.schema, out.Bytes())
			var head struct {
				SchemaVersion int `json:"schema_version"`
			}
			require.NoError(t, json.Unmarshal(out.Bytes(), &head))
			assert.Equal(t, tt.wantVersion, head.SchemaVersion)
		})
	}
}

func TestCatalogV2CarriesLoadCostLintAndExcerpt(t *testing.T) {
	// Arrange
	rolesCmdProject(t)
	resetCatalogFlags(t)
	catalogFormat, catalogSchemaFlag = formatJSON, govview.CatalogSchemaVersionV2
	var out bytes.Buffer

	// Act
	require.NoError(t, runCatalog(&out))

	// Assert
	var doc govview.CatalogDocV2
	require.NoError(t, json.Unmarshal(out.Bytes(), &doc))
	var migrate *govview.CatalogItemV2
	for i := range doc.Items {
		if doc.Items[i].ID == "migrate" {
			migrate = &doc.Items[i]
		}
	}
	require.NotNil(t, migrate)
	assert.Equal(t, "skill/backend/migrate", migrate.Ref)
	assert.Equal(t, "Use when you need migrate.", migrate.Description)
	assert.Equal(t, "local", migrate.Source.Type)
	assert.Positive(t, migrate.LoadCost.ListingTokens)
	assert.Positive(t, migrate.LoadCost.BodyTokens)
	require.NotNil(t, migrate.Lint)
	assert.Contains(t, []string{govview.LintOK, govview.LintWarn, govview.LintError}, migrate.Lint.Status)
	require.NotNil(t, migrate.Excerpt)
	assert.Equal(t, "Body of migrate.\n", migrate.Excerpt.Text)
	assert.True(t, doc.Lint.Available)
	assert.Equal(t, doc.Roles[0].Name, "base")
}

func TestCatalogV2ExcerptOff(t *testing.T) {
	// Arrange
	rolesCmdProject(t)
	resetCatalogFlags(t)
	catalogFormat, catalogSchemaFlag, catalogExcerpt = formatJSON, govview.CatalogSchemaVersionV2, false
	var out bytes.Buffer

	// Act
	require.NoError(t, runCatalog(&out))

	// Assert
	assert.NotContains(t, out.String(), `"excerpt"`)
	validateAgainst(t, "../../schema/catalog.schema.json", out.Bytes())
}

func TestCatalogFlagValidation(t *testing.T) {
	tests := []struct {
		name string
		set  func()
		want string
	}{
		{"unknown schema version", func() { catalogFormat, catalogSchemaFlag = formatJSON, 3 }, "unknown --schema-version"},
		{"html with format", func() { catalogHTMLDir, catalogFormat = "x", formatJSON }, "drop --format"},
		{"role without html", func() { catalogRole = "dev" }, "apply to --html only"},
		{"clean without html", func() { catalogClean = true }, "apply to --html only"},
		{"schema version for text", func() { catalogSchemaFlag = 2 }, "applies to --format json"},
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

// hostileCatalogProject adds items whose text tries to break out of every
// HTML context.
func hostileCatalogProject(t *testing.T) string {
	t.Helper()
	root := rolesCmdProject(t)
	skills := filepath.Join(root, ".ai-rulez", "skills")
	writeFile(t, filepath.Join(skills, "evil", "SKILL.md"),
		"---\nname: evil\ndescription: \"<script>alert(1)</script> \\\"><img src=x onerror=alert(2)> javascript:alert(3)\"\nowner: \"'-alert(4)-'\"\nversion: 1.0.0\n---\n"+
			"Body </script><script>alert(5)</script> <!-- [x](javascript:alert(6)) [y](data:text/html,<b>)\nbidi \u202eevil\u2066 zero\u200bwidth\n")
	if runtime.GOOS != "windows" { // < and > cannot appear in a Windows file name
		writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "<b>bold</b>.md"), "# Rule\nHello\n")
	}
	return root
}

func TestCatalogHTMLEscapesHostileSource(t *testing.T) {
	// Arrange
	hostileCatalogProject(t)
	resetCatalogFlags(t)
	dir := filepath.Join(t.TempDir(), "site")
	catalogHTMLDir = dir
	var out bytes.Buffer

	// Act
	err := runCatalog(&out)

	// Assert
	require.NoError(t, err)
	assert.Contains(t, out.String(), "wrote ")
	files := treeOf(t, dir)
	assert.Contains(t, files, catalogsite.MarkerFile)
	assert.Contains(t, files, "index.html")
	assert.Contains(t, files, "catalog.json")
	assert.Contains(t, files, "robots.txt")
	pages := 0
	for name, page := range files {
		if !strings.HasSuffix(name, ".html") {
			continue
		}
		pages++
		for _, bad := range []string{"<script>alert", "<img src=x", "onerror=alert(2)>", "\u202e", "\u2066", "\u200b", "<b>bold</b>"} {
			assert.NotContainsf(t, page, bad, "%s", name)
		}
		assert.Equalf(t, 1, strings.Count(page, "<script"), "%s: only the local catalog.js script", name)
		bare := quotedValue.ReplaceAllString(page, `=""`) // attribute values are data, not attributes
		assert.NotRegexpf(t, `(?i)<[^>]*\s(on[a-z]+)\s*=`, bare, "%s: no event handler attribute", name)
		assert.NotRegexpf(t, `(?i)(href|src)\s*=\s*["']?\s*(javascript|data|https?):`, page, "%s", name)
	}
	assert.GreaterOrEqual(t, pages, 8)
	var found bool
	for name, page := range files {
		if strings.HasPrefix(name, "items/skill/") && strings.Contains(page, "contains hidden characters") {
			found = true
		}
	}
	assert.True(t, found, "the item with bidi controls is flagged")
}

func TestCatalogHTMLIsByteIdenticalForTheSameInput(t *testing.T) {
	// Arrange
	hostileCatalogProject(t)
	resetCatalogFlags(t)
	a, b := filepath.Join(t.TempDir(), "a"), filepath.Join(t.TempDir(), "b")

	// Act
	catalogHTMLDir = a
	require.NoError(t, runCatalog(&bytes.Buffer{}))
	t.Setenv("TZ", "Asia/Kolkata")
	catalogHTMLDir = b
	require.NoError(t, runCatalog(&bytes.Buffer{}))

	// Assert
	assert.Equal(t, treeOf(t, a), treeOf(t, b))
}

func TestCatalogHTMLDirectoryRules(t *testing.T) {
	// Arrange
	hostileCatalogProject(t)
	resetCatalogFlags(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "precious.txt"), "keep")
	catalogHTMLDir = dir

	// Act
	err := runCatalog(&bytes.Buffer{})
	catalogClean = true
	cleanErr := runCatalog(&bytes.Buffer{})

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), catalogsite.MarkerFile)
	require.Error(t, cleanErr)
	assert.FileExists(t, filepath.Join(dir, "precious.txt"))
	assert.NoFileExists(t, filepath.Join(dir, "index.html"))
}

func TestCatalogHTMLRoleScopeIndexableAndExcerpt(t *testing.T) {
	// Arrange
	rolesCmdProject(t)
	resetCatalogFlags(t)
	dir := filepath.Join(t.TempDir(), "site")
	catalogHTMLDir, catalogRole, catalogIndexable = dir, "dev", true

	// Act
	require.NoError(t, runCatalog(&bytes.Buffer{}))

	// Assert
	files := treeOf(t, dir)
	assert.NotContains(t, files, "robots.txt")
	assert.NotContains(t, files["index.html"], "noindex")
	var doc govview.CatalogDocV2
	require.NoError(t, json.Unmarshal([]byte(files["catalog.json"]), &doc))
	require.Len(t, doc.Roles, 1)
	assert.Equal(t, "dev", doc.Roles[0].Name)
	for _, it := range doc.Items {
		assert.Contains(t, it.Roles, "dev")
		assert.Nil(t, it.Excerpt, "--indexable turns excerpts off unless asked for")
	}
	validateAgainst(t, "../../schema/catalog.schema.json", []byte(files["catalog.json"]))

	// An unknown role is an error, not an empty site.
	catalogRole = "nope"
	require.Error(t, runCatalog(&bytes.Buffer{}))
}

func TestCatalogHTMLRefusesPublishedSecrets(t *testing.T) {
	// Arrange
	root := rolesCmdProject(t)
	resetCatalogFlags(t)
	writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "leak.md"), "# Leak\naws_secret_access_key = \"wJalrXUtnFEMI/K7MDENG/bPxRfiCYQ9d3kGh7Lz\"\nAKIAQYLPMN5HHHFPZAM2\n")
	dir := filepath.Join(t.TempDir(), "site")
	catalogHTMLDir = dir

	// Act
	err := runCatalog(&bytes.Buffer{})

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AR001")
	assert.NoDirExists(t, dir)

	// Allowed explicitly, the site is written.
	catalogAllowFindings = []string{"AR001"}
	require.NoError(t, runCatalog(&bytes.Buffer{}))
	assert.FileExists(t, filepath.Join(dir, "index.html"))
}
