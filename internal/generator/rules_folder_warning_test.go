package generator

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGenerate_RulesFolderWarningIsIssuedOnce pins that the warning about a shared
// AGENTS.md that inlines the rules is said once per run and path, and not at all
// by clean, which says nothing useful while outputs are removed.
func TestGenerate_RulesFolderWarningIsIssuedOnce(t *testing.T) {
	// Arrange
	warnings := quietWarnings(t)
	cfg := "version = \"4.0\"\nname = \"p\"\npresets = [\"junie\", \"codex\"]\ngitignore = false\n"
	root := writeProject(t, cfg, nil)
	count := func() int {
		n := 0
		for _, w := range *warnings {
			if strings.Contains(w, "rules folder") {
				n++
			}
		}
		return n
	}

	// Act
	gen := generateProject(t, root)
	afterGenerate := count()
	_, err := gen.Clean("default", CleanOptions{RemoveEdited: true})
	require.NoError(t, err)

	// Assert
	assert.Equal(t, 1, afterGenerate)
	assert.Equal(t, afterGenerate, count(), "clean issues no further warning")
}
