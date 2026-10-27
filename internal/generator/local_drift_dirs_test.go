package generator

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExclude_OverlayDirectoryOutputsAreMachineLocal(t *testing.T) {
	tests := []struct {
		name      string
		gitignore bool
	}{
		{"managed gitignore", true},
		{"unmanaged gitignore", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange: the overlay adds a preset whose directory outputs the shared preset lacks.
			shared := strings.Replace(driftShared, "gitignore = false", "gitignore = "+strconv.FormatBool(tt.gitignore), 1)
			p := newDriftProject(t, shared)
			p.git(t, "init", "-q")
			p.overlay(t, "presets = [\"claude\", \"gemini\"]\n")

			// Act
			require.NoError(t, NewGenerator(p.load(t)).Generate(""))

			// Assert
			exclude := p.read(t, ".git/info/exclude")
			assert.Contains(t, exclude, "\n/.gemini/agents/\n")
			assert.Contains(t, exclude, "\n/.gemini/\n")
			if tt.gitignore {
				assert.NotContains(t, p.read(t, ".gitignore"), ".gemini")
			}

			// Act: clean still removes the directories and the exclude block.
			_, err := NewGenerator(p.load(t)).Clean("", CleanOptions{})
			require.NoError(t, err)
			assert.False(t, p.exists(".gemini/agents"))
			assert.NotContains(t, p.read(t, ".git/info/exclude"), ".gemini")
		})
	}
}

func TestExclude_DirectoryBothRendersProduceStaysShared(t *testing.T) {
	// Arrange: the shared config already produces .gemini/, the overlay adds codex only.
	shared := strings.Replace(driftShared, "gitignore = false", "gitignore = true", 1)
	shared = strings.Replace(shared, `presets = ["claude"]`, `presets = ["claude", "gemini"]`, 1)
	p := newDriftProject(t, shared)
	p.git(t, "init", "-q")
	p.overlay(t, "presets = [\"claude\", \"gemini\", \"codex\"]\n")

	// Act
	require.NoError(t, NewGenerator(p.load(t)).Generate(""))

	// Assert
	assert.NotContains(t, p.read(t, ".git/info/exclude"), ".gemini")
	assert.Contains(t, p.read(t, ".gitignore"), ".gemini/")
}
