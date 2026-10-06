package commands

import (
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/govview"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefuseSecrets_ScansThePublishedTextItself(t *testing.T) {
	const leak = "aws_secret_access_key = \"wJalrXUtnFEMI/K7MDENG/bPxRfiCYQ9d3kGh7Lz\"\nAKIAQYLPMN5HHHFPZAM2\n"
	tests := []struct {
		name    string
		item    govview.CatalogItemV2
		allow   []string
		wantErr bool
	}{
		{"excerpt secret with no lint report", govview.CatalogItemV2{Ref: "rule:leak", Excerpt: &govview.Excerpt{Text: leak}}, nil, true},
		{"description secret with a clean lint report", govview.CatalogItemV2{Ref: "rule:leak", Description: leak, Lint: &govview.ItemLint{}}, nil, true},
		{"clean item", govview.CatalogItemV2{Ref: "rule:ok", Description: "Use Go.", Excerpt: &govview.Excerpt{Text: "# Go\nUse Go.\n"}}, nil, false},
		{"allowed explicitly", govview.CatalogItemV2{Ref: "rule:leak", Excerpt: &govview.Excerpt{Text: leak}}, []string{"AR001"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			doc := &govview.CatalogDocV2{Items: []govview.CatalogItemV2{tt.item}}

			// Act
			err := refuseSecrets(doc, tt.allow)

			// Assert
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.item.Ref)
				return
			}
			require.NoError(t, err)
		})
	}
}
