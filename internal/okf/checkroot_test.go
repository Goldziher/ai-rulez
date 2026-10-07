package okf

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckRootRequiresVersionedIndex(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{"empty directory", map[string]string{}, []string{"AR9B3 index.md"}},
		{"only a text file", map[string]string{"notes.txt": "hi"}, []string{"AR9B3 index.md"}},
		{"index without okf_version", map[string]string{"index.md": "# Concepts\n"}, []string{"AR9B3 index.md"}},
		{"index with okf_version", map[string]string{"index.md": "---\nokf_version: \"0.2\"\n---\n"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			b := load(t, tt.files)

			// Act
			got := b.CheckRoot()

			// Assert
			assert.Equal(t, tt.want, codes(got))
			for _, f := range got {
				assert.Equal(t, SeverityError, f.Severity)
				assert.Contains(t, f.Message, "okf_version")
			}
		})
	}
}
