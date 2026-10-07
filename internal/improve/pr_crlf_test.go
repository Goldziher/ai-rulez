package improve

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPR_WorksUnderCoreAutocrlf covers TESTGAP-2b: with core.autocrlf=true (the Git for Windows default) the
// worktree checks the skill out with CRLF line endings, which used to read as "the skill changed since the run".
func TestPR_WorksUnderCoreAutocrlf(t *testing.T) {
	tests := []struct {
		name string
		// edit changes the skill on main after the run, "" for none.
		edit    string
		wantErr string
	}{
		{"the measured skill, checked out with CRLF", "", ""},
		{"a real change on the base is still refused", "\nEdited after the run.\n", "AR9J1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			w := newPRWorld(t)
			w.opts.NoPush = true
			skill := filepath.Join(w.configDir, "skills", "deploy", "SKILL.md")
			if tt.edit != "" {
				data, err := os.ReadFile(skill) //nolint:gosec // test file
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(skill, append(data, tt.edit...), 0o600))
				gitIn(t, w.root, "commit", "-q", "-am", "edit")
			}
			global := filepath.Join(t.TempDir(), "gitconfig")
			require.NoError(t, os.WriteFile(global, []byte("[core]\n\tautocrlf = true\n[user]\n\tname = t\n\temail = t@example.test\n[commit]\n\tgpgsign = false\n"), 0o600))
			t.Setenv("GIT_CONFIG_GLOBAL", global)
			t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)

			// Act
			res, err := PR(context.Background(), &w.opts)

			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err, w.out.String())
			files := gitIn(t, w.root, "show", "--name-only", "--format=", res.Branch)
			assert.ElementsMatch(t, []string{".ai-rulez/skills/deploy/SKILL.md", ".ai-rulez/skills/deploy/references/extra.md"}, strings.Split(files, "\n"))
			blob := gitIn(t, w.root, "show", res.Branch+":.ai-rulez/skills/deploy/SKILL.md")
			assert.Contains(t, blob, "GOOD advice.")
			assert.NotContains(t, blob, "\r", "the committed blob keeps LF line endings")
		})
	}
}
