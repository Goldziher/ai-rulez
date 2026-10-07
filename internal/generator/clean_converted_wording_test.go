package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

// cleanWarnings runs clean in dir and returns what it warned.
func cleanWarnings(t *testing.T, dir string) string {
	t.Helper()
	rec := &warnLog{Logger: logger.Discard()}
	cfg, err := config.LoadConfig(t.Context(), dir, config.WithHost(ambient.Host{
		Env: ambient.MapEnv{Home: t.TempDir()}, Log: rec,
	}))
	require.NoError(t, err)
	_, err = NewGenerator(cfg).Clean("default", CleanOptions{})
	require.NoError(t, err)
	return strings.Join(rec.warn, "\n")
}

func TestClean_SaysWhetherAConvertedOriginalWasReplaced(t *testing.T) {
	tests := []struct {
		name     string
		generate bool
		want     string
		notWant  string
	}{
		{"untouched original", false, "it is the file `convert` imported", "it replaced"},
		{"replaced by generate", true, "it replaced a file `convert` imported", "it is the file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quietWarnings(t)
			// Arrange
			dir := hashesProject(t, "")
			claude := filepath.Join(dir, "CLAUDE.md")
			require.NoError(t, os.WriteFile(claude, []byte(handWritten), 0o644))
			require.NoError(t, WriteConvertRecord(filepath.Join(dir, ".ai-rulez"), map[string][]byte{"CLAUDE.md": []byte(handWritten)}))
			if tt.generate {
				require.NoError(t, newProjectGenerator(t, dir).Generate("default"))
			}

			// Act
			joined := cleanWarnings(t, dir)

			// Assert
			assert.Contains(t, joined, tt.want)
			assert.NotContains(t, joined, tt.notWant)
		})
	}
}
