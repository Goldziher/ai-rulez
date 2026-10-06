package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// The lock gives the second item with the same kind, domain and id the key
// "<id>#2"; the catalog must still find the digest of each file.
func TestDigestIndexFindsDisambiguatedDuplicates(t *testing.T) {
	// Arrange
	items := []lockfile.Item{
		{Kind: "rule", ID: "dup", Path: "rules/dup.md", Digest: "sha256:one"},
		{Kind: "rule", ID: "dup#2", Path: "rules/dup.mdc", Digest: "sha256:two"},
		{Kind: "rule", ID: "solo", Digest: "sha256:solo"},
	}
	index := newDigestIndex(items)

	tests := []struct {
		name             string
		kind, domain, id string
		path, want       string
	}{
		{"first duplicate by path", "rule", "", "dup", "rules/dup.md", "sha256:one"},
		{"second duplicate by path", "rule", "", "dup", "rules/dup.mdc", "sha256:two"},
		{"item declared without a path falls back to its key", "rule", "", "solo", "", "sha256:solo"},
		{"unknown item has no digest", "rule", "", "missing", "rules/missing.md", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := index.lookup(tt.kind, tt.domain, tt.id, tt.path)

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}
