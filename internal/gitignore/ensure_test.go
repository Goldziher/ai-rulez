package gitignore

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
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
			name:     "adds to an existing fence keeping its entries, sorted",
			existing: ptr("x\n\n" + BeginMarker + "\nold\n" + EndMarker + "\n\ntail\n"),
			patterns: []string{"new"},
			want:     "x\n\n" + BeginMarker + "\nnew\nold\n" + EndMarker + "\n\ntail\n",
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
			require.NoError(t, EnsureEntries(nil, dir, tt.patterns))
			first, err := os.ReadFile(path)
			require.NoError(t, err)
			require.NoError(t, EnsureEntries(nil, dir, tt.patterns))
			second, err := os.ReadFile(path)
			require.NoError(t, err)

			// Assert
			assert.Equal(t, tt.want, string(first))
			assert.Equal(t, string(first), string(second), "idempotent")
		})
	}
}

func ptr(s string) *string { return &s }

// TestEnsureEntries_NestedConfigWritesTheToplevelBlock is the #517 regression: a
// config below the work tree root contributes prefixed entries to the one block
// at the toplevel, and grows no .gitignore of its own.
func TestEnsureEntries_NestedConfigWritesTheToplevelBlock(t *testing.T) {
	// Arrange
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	require.NoError(t, gitutil.CommandNoContext(root, "init", "-q").Run())
	base := filepath.Join(root, "plugin")
	nested := filepath.Join(base, ".ai-rulez")
	require.NoError(t, os.MkdirAll(nested, 0o755))

	// Act
	require.NoError(t, EnsureEntries(nil, base, []string{".ai-rulez/config.local.*", ".ai-rulez/local/"}))

	// Assert
	toplevel, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	require.NoError(t, err)
	assert.Contains(t, string(toplevel), "plugin/.ai-rulez/config.local.*")
	assert.Contains(t, string(toplevel), "plugin/.ai-rulez/local/")
	_, err = os.Stat(filepath.Join(nested, ".gitignore"))
	assert.True(t, os.IsNotExist(err), "a nested config grows no .gitignore of its own")
}
