package generator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestWriteOutputs_DirectoryMarkers(t *testing.T) {
	tests := []struct {
		name      string
		existing  []string // directories created before the run
		keepFiles []string // files created before the run
		outputs   func(base string) []config.OutputFile
		present   []string
		absent    []string
	}{
		{
			name: "marker without a file is not created",
			outputs: func(base string) []config.OutputFile {
				return []config.OutputFile{
					{Path: filepath.Join(base, ".claude"), IsDir: true},
					{Path: filepath.Join(base, ".claude", "skills"), IsDir: true},
					{Path: filepath.Join(base, ".claude", "agents"), IsDir: true},
					{Path: filepath.Join(base, ".claude", "rules"), IsDir: true},
				}
			},
			absent: []string{".claude"},
		},
		{
			name: "marker with a file is created through the file",
			outputs: func(base string) []config.OutputFile {
				return []config.OutputFile{
					{Path: filepath.Join(base, ".claude", "skills"), IsDir: true},
					{Path: filepath.Join(base, ".claude", "agents"), IsDir: true},
					{Path: filepath.Join(base, ".claude", "agents", "a.md"), Content: "agent\n"},
				}
			},
			present: []string{".claude/agents/a.md"},
			absent:  []string{".claude/skills"},
		},
		{
			name:     "empty directories left by an earlier run are removed",
			existing: []string{".claude/agents", ".claude/skills", ".claude/rules"},
			outputs: func(base string) []config.OutputFile {
				return []config.OutputFile{
					{Path: filepath.Join(base, ".claude"), IsDir: true},
					{Path: filepath.Join(base, ".claude", "skills"), IsDir: true},
					{Path: filepath.Join(base, ".claude", "agents"), IsDir: true},
					{Path: filepath.Join(base, ".claude", "rules"), IsDir: true},
				}
			},
			absent: []string{".claude"},
		},
		{
			name:      "non-empty directories are kept",
			existing:  []string{".claude/rules"},
			keepFiles: []string{".claude/rules/hand.md", ".claude/settings.json"},
			outputs: func(base string) []config.OutputFile {
				return []config.OutputFile{
					{Path: filepath.Join(base, ".claude"), IsDir: true},
					{Path: filepath.Join(base, ".claude", "rules"), IsDir: true},
					{Path: filepath.Join(base, ".claude", "skills"), IsDir: true},
				}
			},
			present: []string{".claude/rules/hand.md", ".claude/settings.json"},
			absent:  []string{".claude/skills"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			base := t.TempDir()
			for _, d := range tt.existing {
				require.NoError(t, os.MkdirAll(filepath.Join(base, d), 0o755))
			}
			for _, f := range tt.keepFiles {
				require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(base, f)), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(base, f), []byte("x"), 0o644))
			}
			gen := NewGenerator(&config.Config{BaseDir: base})

			// Act
			require.NoError(t, gen.writeOutputs(tt.outputs(base)))

			// Assert
			for _, p := range tt.present {
				assert.FileExists(t, filepath.Join(base, p))
			}
			for _, p := range tt.absent {
				assert.NoDirExists(t, filepath.Join(base, p))
			}
		})
	}
}
