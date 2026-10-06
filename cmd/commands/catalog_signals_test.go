package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/evals"
	"github.com/Goldziher/ai-rulez/v5/internal/govview"
)

func catalogItemByID(t *testing.T, doc *govview.CatalogDocV2, kind, id string) *govview.CatalogItemV2 {
	t.Helper()
	for i := range doc.Items {
		if doc.Items[i].Kind == kind && doc.Items[i].ID == id {
			return &doc.Items[i]
		}
	}
	t.Fatalf("no %s %q in the catalog", kind, id)
	return nil
}

func TestCatalogWithEvalAndUsage(t *testing.T) {
	// Arrange
	root := rolesCmdProject(t)
	resetCatalogFlags(t)
	cfgDir := filepath.Join(root, ".ai-rulez")
	store := evals.NewStore()
	store.Put(evals.SkillRecord{ID: "migrate", Passing: true, Date: "2026-10-01", Runner: "private-runner", Score: evals.SkillScore{Scored: 3, PassRate: 1}})
	require.NoError(t, store.Save(filepath.Join(cfgDir, evals.StoreFileName)))
	writeFile(t, filepath.Join(cfgDir, "local", "usage.jsonl"),
		`{"ts":"2026-10-03T10:00:00Z","event":"skill_invoked","skill":"migrate","id":"migrate","invocation":"tool","harness":"claude","session":"s3cr3t"}`+"\n"+
			`{"ts":"2026-10-04T10:00:00Z","event":"skill_invoked","skill":"migrate","id":"migrate","invocation":"slash","harness":"claude"}`+"\n")
	catalogFormat, catalogSchemaFlag = formatJSON, govview.CatalogSchemaVersionV2
	catalogWithEval, catalogWithUsage = catalogDefaultInput, catalogDefaultInput
	var out bytes.Buffer

	// Act
	require.NoError(t, runCatalog(&out))

	// Assert
	validateAgainst(t, "../../schema/catalog.schema.json", out.Bytes())
	var doc govview.CatalogDocV2
	require.NoError(t, json.Unmarshal(out.Bytes(), &doc))
	migrate := catalogItemByID(t, &doc, "skill", "migrate")
	require.NotNil(t, migrate.Eval)
	assert.Equal(t, 1.0, migrate.Eval.PassRate)
	assert.Equal(t, "2026-10-01", migrate.Eval.Date)
	require.NotNil(t, migrate.Usage)
	assert.Equal(t, govview.ItemUsage{Invocations: 2, LastSeen: "2026-10-04"}, *migrate.Usage)
	assert.Nil(t, catalogItemByID(t, &doc, "skill", "deploy").Eval)
	for _, leak := range []string{"private-runner", "s3cr3t"} {
		assert.NotContains(t, out.String(), leak)
	}
}

func TestCatalogWithEvalAndUsageMissingFilesAreNotes(t *testing.T) {
	// Arrange
	rolesCmdProject(t)
	resetCatalogFlags(t)
	catalogFormat, catalogSchemaFlag = formatJSON, govview.CatalogSchemaVersionV2
	catalogWithEval, catalogWithUsage = catalogDefaultInput, filepath.Join(t.TempDir(), "elsewhere.jsonl")
	var out bytes.Buffer

	// Act
	require.NoError(t, runCatalog(&out))

	// Assert
	var doc govview.CatalogDocV2
	require.NoError(t, json.Unmarshal(out.Bytes(), &doc))
	assert.Contains(t, doc.Notes, "eval-results.json not found: eval fields are omitted")
	assert.Contains(t, doc.Notes, "elsewhere.jsonl not found: usage fields are omitted")
	assert.NotContains(t, out.String(), os.TempDir(), "a path of this machine must not reach the catalog")
	for _, it := range doc.Items {
		assert.Nil(t, it.Eval)
		assert.Nil(t, it.Usage)
	}
}

func TestCatalogWithEvalRejectsACorruptFile(t *testing.T) {
	// Arrange
	root := rolesCmdProject(t)
	resetCatalogFlags(t)
	writeFile(t, filepath.Join(root, ".ai-rulez", evals.StoreFileName), "{not json")
	catalogFormat, catalogSchemaFlag, catalogWithEval = formatJSON, govview.CatalogSchemaVersionV2, catalogDefaultInput

	// Act
	err := runCatalog(&bytes.Buffer{})

	// Assert
	require.Error(t, err)
}

func TestCatalogWithFlagsNeedHTMLOrVersion2JSON(t *testing.T) {
	// Arrange
	resetCatalogFlags(t)
	catalogWithEval = catalogDefaultInput

	// Act
	err := checkCatalogFlags()

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--with-eval and --with-usage apply to")
}

func TestCatalogHTMLShowsEvalAndUsageColumns(t *testing.T) {
	// Arrange
	root := rolesCmdProject(t)
	resetCatalogFlags(t)
	cfgDir := filepath.Join(root, ".ai-rulez")
	store := evals.NewStore()
	store.Put(evals.SkillRecord{ID: "migrate", Passing: true, Runner: "r", Score: evals.SkillScore{Scored: 4, PassRate: 0.75}})
	require.NoError(t, store.Save(filepath.Join(cfgDir, evals.StoreFileName)))
	dir := filepath.Join(t.TempDir(), "site")
	catalogHTMLDir, catalogWithEval = dir, catalogDefaultInput

	// Act
	require.NoError(t, runCatalog(&bytes.Buffer{}))

	// Assert
	files := treeOf(t, dir)
	assert.Contains(t, files["index.html"], `<th scope="col">Eval</th>`)
	assert.NotContains(t, files["index.html"], `>Uses<`, "no usage log was asked for")
	assert.Contains(t, files["index.html"], "75%")
	var found bool
	for name, page := range files {
		if filepath.Base(name) == "migrate.html" {
			found = true
			assert.Contains(t, page, "75% over 4 scored case(s)")
		}
	}
	assert.True(t, found)
}
