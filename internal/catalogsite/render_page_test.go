package catalogsite

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/govview"
)

func manyItemsDoc(n int) *govview.CatalogDocV2 {
	doc := &govview.CatalogDocV2{SchemaVersion: 2, GeneratedBy: govview.CatalogGenerator{Name: "ai-rulez", Version: "v"},
		Items: []govview.CatalogItemV2{}, Roles: []govview.CatalogRole{}, Notes: []string{}}
	for i := range n {
		id := "item-" + strconv.Itoa(i)
		doc.Items = append(doc.Items, govview.CatalogItemV2{Ref: "rule/-/" + id, Kind: "rule", ID: id, Source: govview.CatalogSource{Type: "local"},
			Digest: "sha256:" + strings.Repeat("a", 64), Roles: []string{}})
	}
	return doc
}

func TestRender_OverviewIsSplitIntoPages(t *testing.T) {
	tests := []struct {
		name      string
		items     int
		pageSize  int
		wantPages int
	}{
		{"default size keeps a small catalog on one page", 5, 0, 1},
		{"exact multiple", 6, 3, 2},
		{"remainder opens a last page", 7, 3, 3},
		{"page size larger than the catalog", 3, 50, 1},
		{"default is 200", DefaultPageSize + 1, 0, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			doc := manyItemsDoc(tt.items)

			// Act
			site, err := Render(doc, Options{PageSize: tt.pageSize})

			// Assert
			require.NoError(t, err)
			index := string(site.Files["index.html"])
			assert.Equal(t, tt.wantPages, strings.Count(index, "<tbody"))
			assert.Equal(t, tt.items, strings.Count(index, `<tr data-kind=`), "every row is in the page, so browsing works without JavaScript")
			checkHTML(t, "index.html", index)
		})
	}
}

func TestRender_PagingDoesNotChangeTheOtherPages(t *testing.T) {
	// Arrange
	doc := manyItemsDoc(10)

	// Act
	a, errA := Render(doc, Options{PageSize: 3})
	b, errB := Render(doc, Options{PageSize: 100})

	// Assert
	require.NoError(t, errA)
	require.NoError(t, errB)
	assert.Equal(t, a.Digest, b.Digest, "the page size is a presentation detail: catalog.json is the same")
	for name := range a.Files {
		if name != "index.html" {
			assert.Equal(t, string(a.Files[name]), string(b.Files[name]), name)
		}
	}
}
