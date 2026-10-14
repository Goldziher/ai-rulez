package agentssource

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

const sectionsText = "# Demo\n\nIntro.\n\n## Setup\n\nRun it.\n\n## Style\n\nTabs.\n"

func writeAgentsMD(t *testing.T, content string, mode os.FileMode) (base, path string) {
	t.Helper()
	base = t.TempDir()
	path = filepath.Join(base, "AGENTS.md")
	require.NoError(t, os.WriteFile(path, []byte(content), mode))
	require.NoError(t, os.Chmod(path, mode))
	return base, path
}

func TestRemoveItemKeepsTheBytesAndTheMode(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{"LF", sectionsText, "# Demo\n\nIntro.\n\n## Style\n\nTabs.\n"},
		{"CRLF", "# Demo\r\n\r\nIntro.\r\n\r\n## Setup\r\n\r\nRun it.\r\n\r\n## Style\r\n\r\nTabs.\r\n",
			"# Demo\r\n\r\nIntro.\r\n\r\n## Style\r\n\r\nTabs.\r\n"},
		{"BOM", "\ufeff## Setup\n\nRun it.\n\n## Style\n\nTabs.\n", "\ufeff## Style\n\nTabs.\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			base, path := writeAgentsMD(t, tt.text, 0o600)

			// Act
			err := RemoveItem(base, "agents-setup", true, nil)

			// Assert
			require.NoError(t, err)
			got, readErr := os.ReadFile(path)
			require.NoError(t, readErr)
			assert.Equal(t, tt.want, string(got))
			if runtime.GOOS != "windows" {
				info, statErr := os.Stat(path)
				require.NoError(t, statErr)
				assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "the mode is kept")
			}
			entries, dirErr := os.ReadDir(base)
			require.NoError(t, dirErr)
			assert.Len(t, entries, 1, "no temporary file is left behind")
		})
	}
}

func TestRemoveItemRefusesALoneCarriageReturn(t *testing.T) {
	// Arrange
	text := "# Demo\r\r## Setup\rRun it.\r## Style\rTabs.\r"
	base, path := writeAgentsMD(t, text, 0o644)

	// Act
	err := RemoveItem(base, "agents-setup", true, nil)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "line endings")
	got, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, text, string(got))
}

func TestRemoveItemRefusesASymlinkedFile(t *testing.T) {
	// Arrange
	base := t.TempDir()
	target := filepath.Join(base, "elsewhere.md")
	require.NoError(t, os.WriteFile(target, []byte(sectionsText), 0o644))
	testutil.SymlinkOrSkip(t, "elsewhere.md", filepath.Join(base, "AGENTS.md"))

	// Act
	err := RemoveItem(base, "agents-setup", true, nil)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlink")
	got, readErr := os.ReadFile(target)
	require.NoError(t, readErr)
	assert.Equal(t, sectionsText, string(got), "the link target is untouched")
}

func TestRemoveItemDetectsAConcurrentEdit(t *testing.T) {
	// Arrange
	base, path := writeAgentsMD(t, sectionsText, 0o644)
	edited := sectionsText + "\nAdded by someone else.\n"

	// Act
	err := removeItem(base, "agents-setup", true, nil, func() {
		require.NoError(t, os.WriteFile(path, []byte(edited), 0o644))
	})

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "changed")
	got, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, edited, string(got), "the concurrent edit is not overwritten")
	entries, dirErr := os.ReadDir(base)
	require.NoError(t, dirErr)
	assert.Len(t, entries, 1, "no temporary file is left behind")
}
