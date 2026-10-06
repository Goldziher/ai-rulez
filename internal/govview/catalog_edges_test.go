package govview

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func edgeItem(kind, domain, id string) CatalogItemV2 {
	d := domain
	if d == "" {
		d = "-"
	}
	return CatalogItemV2{Ref: kind + "/" + d + "/" + id, Kind: kind, Domain: domain, ID: id}
}

func uses(skills ...string) *config.ContentFile {
	return &config.ContentFile{Metadata: &config.Metadata{Skills: skills}}
}

func TestBuildEdges(t *testing.T) {
	items := []CatalogItemV2{
		edgeItem("agent", "", "reviewer"),     // 0
		edgeItem("skill", "", "lint"),         // 1
		edgeItem("skill", "ops", "deploy"),    // 2
		edgeItem("skill", "ops", "rollback"),  // 3
		edgeItem("skill", "dev", "rollback"),  // 4
		edgeItem("skill", "", "rollback"),     // 5 (root wins over a domain when the caller has none)
		edgeItem("rule", "", "style"),         // 6
		edgeItem("skill", "dev", "unrelated"), // 7
	}
	tests := []struct {
		name      string
		files     map[int]*config.ContentFile
		wantEdges []CatalogEdge
		wantNotes []string
	}{
		{
			name:  "agent to skills and skill to skill",
			files: map[int]*config.ContentFile{0: uses("lint", "deploy"), 2: uses("lint")},
			wantEdges: []CatalogEdge{
				{From: "agent/-/reviewer", To: "skill/-/lint", Kind: "uses"},
				{From: "agent/-/reviewer", To: "skill/ops/deploy", Kind: "uses"},
				{From: "skill/ops/deploy", To: "skill/-/lint", Kind: "uses"},
			},
		},
		{
			name:      "same domain wins when the name is shared",
			files:     map[int]*config.ContentFile{3: uses("rollback"), 4: uses("rollback"), 7: uses("rollback")},
			wantEdges: []CatalogEdge{{From: "skill/dev/unrelated", To: "skill/dev/rollback", Kind: "uses"}},
			wantNotes: nil,
		},
		{
			name:      "an unknown skill is noted, not drawn",
			files:     map[int]*config.ContentFile{6: uses("ghost")},
			wantEdges: []CatalogEdge{},
			wantNotes: []string{"rule/-/style names skill ghost, which is not in the catalog: the dependency is not drawn"},
		},
		{
			name:      "self references and duplicates collapse",
			files:     map[int]*config.ContentFile{1: uses("lint"), 6: uses("lint", "lint", " lint ", "")},
			wantEdges: []CatalogEdge{{From: "rule/-/style", To: "skill/-/lint", Kind: "uses"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			files := make([]*config.ContentFile, len(items))
			for i, f := range tt.files {
				files[i] = f
			}

			// Act
			edges, notes := buildEdges(items, files)

			// Assert
			assert.Equal(t, tt.wantEdges, edges)
			assert.Equal(t, tt.wantNotes, notes)
		})
	}
}

func TestBuildEdges_AmbiguousNameIsNotGuessed(t *testing.T) {
	// Arrange: two non-root skills of the same name, a caller in a third domain
	items := []CatalogItemV2{edgeItem("agent", "web", "a"), edgeItem("skill", "ops", "x"), edgeItem("skill", "dev", "x")}
	files := []*config.ContentFile{uses("x"), nil, nil}

	// Act
	edges, notes := buildEdges(items, files)

	// Assert
	assert.Empty(t, edges)
	assert.Equal(t, []string{"agent/web/a names skill x, which exists in several domains: the dependency is not drawn"}, notes)
}

func TestBuildEdges_IsSortedAndDeterministic(t *testing.T) {
	// Arrange
	items := []CatalogItemV2{edgeItem("agent", "", "z"), edgeItem("agent", "", "a"), edgeItem("skill", "", "s")}
	files := []*config.ContentFile{uses("s"), uses("s"), nil}

	// Act
	edges, _ := buildEdges(items, files)

	// Assert
	assert.Equal(t, []CatalogEdge{{From: "agent/-/a", To: "skill/-/s", Kind: "uses"}, {From: "agent/-/z", To: "skill/-/s", Kind: "uses"}}, edges)
}

func TestViewCatalogV2_KeepsOnlyEdgesBetweenKeptItems(t *testing.T) {
	// Arrange
	doc := diffDoc(
		CatalogItemV2{Ref: "agent/-/a", Roles: []string{"dev"}},
		CatalogItemV2{Ref: "skill/-/s", Roles: []string{"dev"}},
		CatalogItemV2{Ref: "skill/-/t", Roles: []string{"ops"}})
	doc.Roles = []CatalogRole{{Name: "dev"}, {Name: "ops"}}
	doc.Edges = []CatalogEdge{{From: "agent/-/a", To: "skill/-/s", Kind: "uses"}, {From: "agent/-/a", To: "skill/-/t", Kind: "uses"}}

	// Act
	got, err := ViewCatalogV2(doc, "dev")

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, []CatalogEdge{{From: "agent/-/a", To: "skill/-/s", Kind: "uses"}}, got.Edges)
	assert.Len(t, doc.Edges, 2, "the input is not modified")
}

func TestDiffCatalogs_Edges(t *testing.T) {
	// Arrange
	from, to := diffDoc(), diffDoc()
	from.Edges = []CatalogEdge{{From: "a", To: "b", Kind: "uses"}, {From: "a", To: "c", Kind: "uses"}}
	to.Edges = []CatalogEdge{{From: "a", To: "b", Kind: "uses"}, {From: "a", To: "d", Kind: "uses"}}

	// Act
	got, err := DiffCatalogs(from, to, DiffSide{}, DiffSide{})

	// Assert
	assert.NoError(t, err)
	assert.False(t, got.Identical)
	assert.Equal(t, []DiffEntry{{Ref: "a -> d", Kind: "uses"}}, got.Edges.Added)
	assert.Equal(t, []DiffEntry{{Ref: "a -> c", Kind: "uses"}}, got.Edges.Removed)
}
