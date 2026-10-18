package generator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// A hand-written skill that has the name of a generated builtin skill is the
// project's own: generate refuses to overwrite it (and names the file) instead
// of replacing it with the builtin.
func TestGenerate_HandWrittenSkillIsNotClobberedByBuiltin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		rel  string
	}{
		{name: "builtin testing skill", rel: ".claude/skills/testing-conventions/SKILL.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Arrange
			dir := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(dir, ".ai-rulez"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", "config.toml"),
				[]byte("version = \"5.0\"\nname = \"x\"\npresets = [\"claude\"]\nbuiltins = [\"testing\"]\n"), 0o644))
			abs := filepath.Join(dir, filepath.FromSlash(tt.rel))
			require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
			const mine = "---\nname: testing-conventions\ndescription: Use when writing tests here.\n---\nmine\n"
			require.NoError(t, os.WriteFile(abs, []byte(mine), 0o644))
			cfg, err := config.LoadConfig(context.Background(), dir)
			require.NoError(t, err)

			// Act
			err = NewGenerator(cfg).Generate("")

			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), "testing-conventions/SKILL.md")
			data, readErr := os.ReadFile(abs)
			require.NoError(t, readErr)
			assert.Equal(t, mine, string(data))
		})
	}
}
