package govview

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOmitOwners(t *testing.T) {
	// Arrange
	doc := &CatalogDocV2{Items: []CatalogItemV2{{ID: "a", Owner: "team-a"}, {ID: "b"}}, Notes: []string{"n"}}

	// Act
	got := OmitOwners(doc)

	// Assert
	assert.Empty(t, got.Items[0].Owner)
	assert.Equal(t, "a", got.Items[0].ID)
	assert.Equal(t, "team-a", doc.Items[0].Owner, "the input is not modified")
	assert.Equal(t, []string{"n", "owners were switched off: item owners are omitted"}, got.Notes)
	assert.Equal(t, []string{"n"}, doc.Notes)
}
