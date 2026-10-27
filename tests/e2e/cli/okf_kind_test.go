package cli

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/tests/e2e/testutil"
)

// TestOKFConceptKindIsValidated: an unknown x-ai-rulez.kind, or a known one that
// disagrees with the concept's directory, is reported by validate --strict and
// okf validate instead of passing silently (issue #509).
func TestOKFConceptKindIsValidated(t *testing.T) {
	t.Cleanup(testutil.CleanupTestBinary)
	tests := []struct {
		name     string
		kind     string
		wantExit int
		wantText string
	}{
		{"unknown kind", "bogus", 2, "use rules, context, skills, agents, commands, checks or skill-resource"},
		{"kind disagrees with directory", "skill", 2, "disagrees with its directory context/"},
		{"matching kind", "context", 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := testutil.CreateTempDir(t)
			gitInitAgents(t, dir)
			cfg := filepath.Join(dir, ".ai-rulez")
			putFile(t, cfg, "config.toml", "version = \"5.0\"\nname = \"i1\"\npresets = [\"claude\"]\n")
			putFile(t, cfg, "index.md", "---\nokf_version: \"0.2\"\n---\n\n# Subdirectories\n\n* [context](context/index.md) - Context\n")
			putFile(t, filepath.Join(cfg, "context"), "index.md", "# Concepts\n\n* [Note](note.md)\n")
			putFile(t, filepath.Join(cfg, "context"), "note.md",
				"---\ntype: Concept\ntitle: Note\nx-ai-rulez:\n  kind: "+tt.kind+"\n  id: note\n---\n\n# Note\n\nSome context.\n")

			testutil.RunCLIExpectSuccess(t, dir, "generate", "--no-local", "--yes")

			for _, args := range [][]string{
				{"validate", "--strict", "--no-local"},
				{"okf", "validate", ".ai-rulez", "--fail-on", "warning"},
			} {
				// Act
				res := testutil.RunCLI(t, dir, args...)

				// Assert
				out := res.Stdout + res.Stderr
				require.Equal(t, tt.wantExit, res.ExitCode, "%v: %s", args, out)
				if tt.wantText != "" {
					assert.Contains(t, out, "AR9B1", "%v", args)
					assert.Contains(t, out, tt.wantText, "%v", args)
				}
			}
		})
	}
}
