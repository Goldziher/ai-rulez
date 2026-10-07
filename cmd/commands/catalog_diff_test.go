package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/govview"
)

func resetCatalogDiffFlags(t *testing.T) {
	t.Helper()
	resetCatalogFlags(t)
	t.Cleanup(func() { catalogDiffFormat, catalogDiffExitCode = "", false })
}

// catalogDiffRepo commits the roles fixture, then changes a skill, removes a
// rule and adds a rule and an MCP server in a second commit (left committed).
func catalogDiffRepo(t *testing.T) (root string) {
	t.Helper()
	root = rolesCmdProject(t)
	t.Setenv("HOME", t.TempDir())
	gitIn(t, root, "init", "-q", "-b", "main")
	gitIn(t, root, "add", "-A")
	gitIn(t, root, "commit", "-q", "-m", "one")
	writeFile(t, filepath.Join(root, ".ai-rulez", "domains", "backend", "skills", "migrate", "SKILL.md"),
		"---\nname: migrate\ndescription: Use when you need migrate. Rewritten.\n---\nA longer body of migrate, with more words in it.\n")
	require.NoError(t, os.Remove(filepath.Join(root, ".ai-rulez", "rules", "style.md")))
	writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "fresh.md"), "# Fresh\nNew rule.\n")
	cfgPath := filepath.Join(root, ".ai-rulez", "config.toml")
	data, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	writeFile(t, cfgPath, string(data)+"\n[[mcp_servers]]\nname = \"github\"\ncommand = \"npx\"\nargs = [\"-y\", \"pkg@1.2.3\"]\n")
	gitIn(t, root, "add", "-A")
	gitIn(t, root, "commit", "-q", "-m", "two")
	return root
}

func TestCatalogDiffBetweenRevisions(t *testing.T) {
	// Arrange
	catalogDiffRepo(t)
	resetCatalogDiffFlags(t)
	catalogDiffFormat = formatJSON
	var out bytes.Buffer

	// Act
	identical, err := runCatalogDiff(context.Background(), &out, []string{"HEAD~1", "HEAD"})

	// Assert
	require.NoError(t, err)
	assert.False(t, identical)
	validateAgainst(t, "../../schema/catalog-diff.schema.json", out.Bytes())
	var diff govview.CatalogDiff
	require.NoError(t, json.Unmarshal(out.Bytes(), &diff))
	assert.Equal(t, "HEAD~1", diff.From.Label)
	assert.Len(t, diff.From.Commit, 40)
	assert.NotEqual(t, diff.From.Commit, diff.To.Commit)
	require.Len(t, diff.Items.Added, 1)
	assert.Equal(t, "rule/-/fresh", diff.Items.Added[0].Ref)
	require.Len(t, diff.Items.Removed, 1)
	assert.Equal(t, "rule/-/style", diff.Items.Removed[0].Ref)
	require.Len(t, diff.Items.Changed, 1)
	assert.Equal(t, "skill/backend/migrate", diff.Items.Changed[0].Ref)
	fields := []string{}
	for _, c := range diff.Items.Changed[0].Changes {
		fields = append(fields, c.Field)
	}
	assert.Contains(t, fields, "digest")
	assert.Contains(t, fields, "description")
	assert.Contains(t, fields, "body_tokens")
	require.Len(t, diff.MCPServers.Added, 1)
	assert.Equal(t, "mcp/github", diff.MCPServers.Added[0].Ref)
}

func TestCatalogDiffWorkingTreeAgainstARevision(t *testing.T) {
	// Arrange
	root := catalogDiffRepo(t)
	resetCatalogDiffFlags(t)
	writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "wip.md"), "# WIP\nUncommitted.\n")
	writeFile(t, filepath.Join(root, ".ai-rulez", "local", "rules", "mine.md"), "# Mine\nMachine-local.\n")
	var out bytes.Buffer

	// Act
	identical, err := runCatalogDiff(context.Background(), &out, []string{"HEAD"})

	// Assert
	require.NoError(t, err)
	assert.False(t, identical)
	assert.Contains(t, out.String(), "+ rule/-/wip")
	assert.NotContains(t, out.String(), "mine", "the machine-local overlay is not part of either side")
	assert.Contains(t, out.String(), "-> working tree")
}

func TestCatalogDiffIdenticalRevisions(t *testing.T) {
	// Arrange
	catalogDiffRepo(t)
	resetCatalogDiffFlags(t)
	var out bytes.Buffer

	// Act
	identical, err := runCatalogDiff(context.Background(), &out, []string{"HEAD", "HEAD"})

	// Assert
	require.NoError(t, err)
	assert.True(t, identical)
	assert.Contains(t, out.String(), "no differences")
}

func TestCatalogDiffOfTwoJSONFiles(t *testing.T) {
	// Arrange
	catalogDiffRepo(t)
	resetCatalogDiffFlags(t)
	dir := t.TempDir()
	for name, rev := range map[string]string{"before.json": "HEAD~1", "after.json": "HEAD"} {
		catalogFormat, catalogSchemaFlag = formatJSON, govview.CatalogSchemaVersionV2
		var buf bytes.Buffer
		// The files are written from the committed state through the revision snapshot path.
		side, err := revisionSide(context.Background(), &catalogDiffProject{}, rev)
		require.NoError(t, err)
		require.NoError(t, writeJSON(&buf, side.doc))
		writeFile(t, filepath.Join(dir, name), buf.String())
	}
	catalogFormat, catalogSchemaFlag = "", govview.CatalogSchemaVersion
	var out bytes.Buffer

	// Act
	identical, err := runCatalogDiff(context.Background(), &out, []string{filepath.Join(dir, "before.json"), filepath.Join(dir, "after.json")})

	// Assert
	require.NoError(t, err)
	assert.False(t, identical)
	assert.Contains(t, out.String(), "before.json -> after.json")
	assert.Contains(t, out.String(), "+ rule/-/fresh")
	assert.Contains(t, out.String(), "- rule/-/style")
	assert.Contains(t, out.String(), "~ skill/backend/migrate")
	assert.Contains(t, out.String(), "+ mcp/github")
}

func TestCatalogDiffRefusals(t *testing.T) {
	// Arrange
	catalogDiffRepo(t)
	resetCatalogDiffFlags(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "v1.json"), `{"schema_version": 1, "items": []}`)
	writeFile(t, filepath.Join(dir, "junk.json"), `not json`)
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"unknown revision", []string{"no-such-rev"}, "does not exist"},
		{"option as revision", []string{"--output=x"}, "git option"},
		{"version 1 file", []string{filepath.Join(dir, "v1.json"), "HEAD"}, "schema_version 1 is not supported"},
		{"not json", []string{filepath.Join(dir, "junk.json"), "HEAD"}, "not a catalog document"},
		{"missing json file", []string{filepath.Join(dir, "gone.json"), "HEAD"}, "was not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			_, err := runCatalogDiff(context.Background(), &bytes.Buffer{}, tt.args)

			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestCatalogDiffTextNeverEmitsControlCharacters(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	mk := func(desc string) string {
		doc := &govview.CatalogDocV2{SchemaVersion: govview.CatalogSchemaVersionV2, MCPServers: []govview.CatalogMCPServer{},
			Items: []govview.CatalogItemV2{{Ref: "skill/-/x\x1b[31m", Kind: "skill", ID: "x", Description: desc}}, Lint: govview.CatalogLint{Available: true}}
		data, err := json.Marshal(doc)
		require.NoError(t, err)
		return string(data)
	}
	writeFile(t, filepath.Join(dir, "a.json"), mk("one"))
	writeFile(t, filepath.Join(dir, "b.json"), mk("two\x1b]0;pwned\x07 \u202eevil"))
	resetCatalogDiffFlags(t)
	var out bytes.Buffer

	// Act
	_, err := runCatalogDiff(context.Background(), &out, []string{filepath.Join(dir, "a.json"), filepath.Join(dir, "b.json")})

	// Assert
	require.NoError(t, err)
	for _, r := range out.String() {
		assert.False(t, r != '\n' && r != '\t' && (r < 0x20 || r == 0x7f), "control character %U in the output", r)
	}
	assert.NotContains(t, out.String(), "\u202e")
	assert.True(t, strings.Contains(out.String(), `\u{1B}`))
}
