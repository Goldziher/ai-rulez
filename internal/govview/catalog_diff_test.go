package govview

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/roles"
)

func diffDoc(items ...CatalogItemV2) *CatalogDocV2 {
	return &CatalogDocV2{SchemaVersion: CatalogSchemaVersionV2, Items: items, MCPServers: []CatalogMCPServer{}, Roles: []CatalogRole{},
		Lint: CatalogLint{Available: true}, Notes: []string{}}
}

func diffItem(ref, digest string, body int) CatalogItemV2 {
	return CatalogItemV2{Ref: ref, Kind: "skill", ID: ref, Digest: digest, LoadCost: LoadCost{ListingTokens: 5, BodyTokens: body}, Roles: []string{}}
}

func TestDiffCatalogs_Items(t *testing.T) {
	// Arrange
	changedBefore := diffItem("skill/-/b", "sha256:1", 100)
	changedBefore.Description = "old"
	changedBefore.Lint = &ItemLint{Status: LintOK}
	changedBefore.Roles = []string{"dev"}
	changedAfter := diffItem("skill/-/b", "sha256:2", 130)
	changedAfter.Description = "new"
	changedAfter.Lint = &ItemLint{Status: LintWarn}
	changedAfter.Roles = []string{"dev", "ops"}
	from := diffDoc(diffItem("skill/-/gone", "sha256:g", 10), changedBefore, diffItem("skill/-/same", "sha256:s", 7))
	to := diffDoc(diffItem("skill/-/new", "sha256:n", 40), changedAfter, diffItem("skill/-/same", "sha256:s", 7))

	// Act
	got, err := DiffCatalogs(from, to, DiffSide{Label: "a"}, DiffSide{Label: "b"})

	// Assert
	require.NoError(t, err)
	assert.False(t, got.Identical)
	assert.Equal(t, []DiffEntry{{Ref: "skill/-/new", Kind: "skill", Tokens: 45}}, got.Items.Added)
	assert.Equal(t, []DiffEntry{{Ref: "skill/-/gone", Kind: "skill", Tokens: 15}}, got.Items.Removed)
	require.Len(t, got.Items.Changed, 1)
	assert.Equal(t, "skill/-/b", got.Items.Changed[0].Ref)
	assert.Equal(t, []DiffChange{
		{Field: "digest", From: "sha256:1", To: "sha256:2"},
		{Field: "description", From: "old", To: "new"},
		{Field: "body_tokens", From: "100", To: "130"},
		{Field: "lint", From: "ok", To: "warn"},
		{Field: "roles", From: "dev", To: "dev, ops"},
	}, got.Items.Changed[0].Changes)
	assert.Equal(t, 3, got.From.Items)
	assert.Equal(t, 117, got.From.BodyTokens)
	assert.Equal(t, 177, got.To.BodyTokens)
}

func TestDiffCatalogs_IdenticalAndDeterministic(t *testing.T) {
	// Arrange
	a := diffDoc(diffItem("skill/-/a", "sha256:1", 1), diffItem("skill/-/b", "sha256:2", 2))
	b := diffDoc(diffItem("skill/-/b", "sha256:2", 2), diffItem("skill/-/a", "sha256:1", 1)) // another order

	// Act
	got, err := DiffCatalogs(a, b, DiffSide{Label: "a"}, DiffSide{Label: "b"})

	// Assert
	require.NoError(t, err)
	assert.True(t, got.Identical)
	assert.True(t, got.Items.Empty() && got.MCPServers.Empty() && got.Roles.Empty())
}

func TestDiffCatalogs_MCPServersAndRoles(t *testing.T) {
	// Arrange
	no, yes := false, true
	from := diffDoc()
	from.MCPServers = []CatalogMCPServer{
		{Ref: "mcp/gh", Name: "gh", Transport: "stdio", CommandBasename: "npx", Pinned: &no, Enabled: true, Env: []MCPValue{{Name: "TOKEN", Ref: "TOKEN"}}},
		{Ref: "mcp/old", Name: "old", Transport: "http"},
	}
	from.Roles = []CatalogRole{{Name: "dev", Domains: []string{"a"}, Totals: roles.Totals{Items: 2, Tokens: 10}}, {Name: "gone"}}
	to := diffDoc()
	to.MCPServers = []CatalogMCPServer{
		{Ref: "mcp/gh", Name: "gh", Transport: "stdio", CommandBasename: "npx", Pinned: &yes, Enabled: true, Env: []MCPValue{{Name: "TOKEN", Literal: true}}},
		{Ref: "mcp/new", Name: "new", Transport: "sse"},
	}
	to.Roles = []CatalogRole{{Name: "dev", Domains: []string{"a", "b"}, Totals: roles.Totals{Items: 3, Tokens: 25}}, {Name: "fresh"}}

	// Act
	got, err := DiffCatalogs(from, to, DiffSide{}, DiffSide{})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, []DiffEntry{{Ref: "mcp/new", Kind: "mcp"}}, got.MCPServers.Added)
	assert.Equal(t, []DiffEntry{{Ref: "mcp/old", Kind: "mcp"}}, got.MCPServers.Removed)
	require.Len(t, got.MCPServers.Changed, 1)
	assert.Equal(t, []DiffChange{
		{Field: "pinned", From: "unpinned", To: "pinned"},
		{Field: "env", From: "TOKEN (${TOKEN})", To: "TOKEN (literal)"},
	}, got.MCPServers.Changed[0].Changes)
	assert.Equal(t, []DiffEntry{{Ref: "fresh", Kind: "role"}}, got.Roles.Added)
	assert.Equal(t, []DiffEntry{{Ref: "gone", Kind: "role"}}, got.Roles.Removed)
	require.Len(t, got.Roles.Changed, 1)
	assert.Equal(t, []DiffChange{
		{Field: "domains", From: "a", To: "a, b"},
		{Field: "items", From: "2", To: "3"},
		{Field: "tokens", From: "10", To: "25"},
	}, got.Roles.Changed[0].Changes)
}

func TestDiffCatalogs_LintTotalsAndMissingLint(t *testing.T) {
	// Arrange
	from, to := diffDoc(), diffDoc()
	to.Lint.Summary.Warnings = 2

	// Act
	got, err := DiffCatalogs(from, to, DiffSide{}, DiffSide{})
	to.Lint.Available = false
	skipped, skipErr := DiffCatalogs(from, to, DiffSide{}, DiffSide{})

	// Assert
	require.NoError(t, err)
	require.NotNil(t, got.Lint)
	assert.False(t, got.Identical, "lint totals differ")
	assert.Equal(t, 2, got.Lint.To.Warnings)
	require.NoError(t, skipErr)
	assert.Nil(t, skipped.Lint)
	assert.Contains(t, skipped.Notes[0], "lint totals are not compared")
}

func TestDiffCatalogs_RefusesOtherSchemaVersions(t *testing.T) {
	// Arrange
	good := diffDoc()
	v1 := diffDoc()
	v1.SchemaVersion = 1

	// Act
	_, errFrom := DiffCatalogs(v1, good, DiffSide{}, DiffSide{})
	_, errTo := DiffCatalogs(good, nil, DiffSide{}, DiffSide{})

	// Assert
	require.Error(t, errFrom)
	assert.Contains(t, errFrom.Error(), "version 2")
	require.Error(t, errTo)
}

func TestDiffCatalogs_JSONHasNoNullSections(t *testing.T) {
	// Arrange / Act
	got, err := DiffCatalogs(diffDoc(), diffDoc(), DiffSide{Label: "a"}, DiffSide{Label: "b"})
	require.NoError(t, err)
	data, err := json.Marshal(got)

	// Assert
	require.NoError(t, err)
	assert.Contains(t, string(data), `"added":[]`)
	assert.NotContains(t, string(data), `"added":null`)
}
