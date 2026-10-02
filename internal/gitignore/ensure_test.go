package gitignore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureEntries(t *testing.T) {
	tests := []struct {
		name     string
		existing *string
		patterns []string
		want     string
	}{
		{
			name:     "creates the file with a fence",
			patterns: []string{".ai-rulez/config.local.*"},
			want:     BeginMarker + "\n.ai-rulez/config.local.*\n" + EndMarker + "\n",
		},
		{
			name:     "appends a fence after existing lines",
			existing: ptr("node_modules/\n"),
			patterns: []string{"a"},
			want:     "node_modules/\n\n" + BeginMarker + "\na\n" + EndMarker + "\n",
		},
		{
			name:     "adds to an existing fence keeping its entries",
			existing: ptr("x\n\n" + BeginMarker + "\nold\n" + EndMarker + "\n\ntail\n"),
			patterns: []string{"new"},
			want:     "x\n\n" + BeginMarker + "\nold\nnew\n" + EndMarker + "\n\ntail\n",
		},
		{
			name:     "fence without END is not absorbed; a fresh complete fence is appended",
			existing: ptr("keep-me\n" + BeginMarker + "\nold\nuser-line\n"),
			patterns: []string{"new"},
			want:     "keep-me\n" + BeginMarker + "\nold\nuser-line\n\n" + BeginMarker + "\nnew\n" + EndMarker + "\n",
		},
		{
			name:     "pattern already in fence is untouched",
			existing: ptr(BeginMarker + "\na\n" + EndMarker + "\n"),
			patterns: []string{"a"},
			want:     BeginMarker + "\na\n" + EndMarker + "\n",
		},
		{
			name:     "pattern already outside the fence is untouched",
			existing: ptr("a\n"),
			patterns: []string{"a"},
			want:     "a\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			path := filepath.Join(dir, ".gitignore")
			if tt.existing != nil {
				require.NoError(t, os.WriteFile(path, []byte(*tt.existing), 0o600))
			}

			// Act
			require.NoError(t, EnsureEntries(dir, tt.patterns))
			first, err := os.ReadFile(path)
			require.NoError(t, err)
			require.NoError(t, EnsureEntries(dir, tt.patterns))
			second, err := os.ReadFile(path)
			require.NoError(t, err)

			// Assert
			assert.Equal(t, tt.want, string(first))
			assert.Equal(t, string(first), string(second), "idempotent")
		})
	}
}

func ptr(s string) *string { return &s }
