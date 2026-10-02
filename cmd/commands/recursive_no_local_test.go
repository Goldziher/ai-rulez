package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRunRecursiveGenerate_NoLocalGeneratesTheSharedView(t *testing.T) {
	tests := []struct {
		name        string
		noLocal     bool
		wantLocalAI bool
	}{
		{"with local config", false, true},
		{"with --no-local", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange: root b has an overlay that adds the codex preset (AGENTS.md).
			root := twoRoots(t, validRootConfig)
			writeFile(t, filepath.Join(root, "b", ".ai-rulez", "config.local.toml"), "presets = [\"codex\"]\n")
			noLocal = tt.noLocal
			t.Cleanup(func() { noLocal = false })

			// Act
			code := runRecursiveGenerate()

			// Assert
			assert.Equal(t, 0, code)
			_, err := os.Stat(filepath.Join(root, "b", "AGENTS.md"))
			assert.Equal(t, tt.wantLocalAI, err == nil)
			_, err = os.Stat(filepath.Join(root, "b", "CLAUDE.md"))
			assert.NoError(t, err, "shared outputs are generated either way")
		})
	}
}
