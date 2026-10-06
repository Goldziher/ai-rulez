package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApprove_VerifyBaseReadsOwnersFromTheBase(t *testing.T) {
	// Arrange: bob is made an owner and approves in the same range
	root := approveProject(t, "approvers_from = \"CODEOWNERS\"\n")
	writeFile(t, filepath.Join(root, "CODEOWNERS"), "* @alice\n")
	crossGit(t, root, "init", "-q", "-b", "main")
	crossGit(t, root, "add", "-A")
	crossGit(t, root, "commit", "-qm", "base")
	writeFile(t, filepath.Join(root, "CODEOWNERS"), "* @alice @bob\n")
	approveYes, approveReviewer = true, "bob"
	require.Equal(t, 0, mustApprove(t, "rule:style"))
	resetApproveFlags()
	approveVerifyBase = "main"

	// Act
	code, stdout, stderr := runApproveCmd(t)

	// Assert
	assert.Equal(t, 2, code, stderr)
	assert.Contains(t, stdout, "not by an owner of it in CODEOWNERS at")
}

func TestApprove_VerifyBaseFlagsAChangeToCodeownersAndGovernance(t *testing.T) {
	tests := []struct {
		name   string
		change func(t *testing.T, root string)
		want   string
	}{
		{"CODEOWNERS edited in the range", func(t *testing.T, root string) {
			writeFile(t, filepath.Join(root, "CODEOWNERS"), "* @alice @mallory\n")
		}, "CODEOWNERS changed"},
		{"governance table edited in the range", func(t *testing.T, root string) {
			p := filepath.Join(root, ".ai-rulez", "config.toml")
			data, err := os.ReadFile(p)
			require.NoError(t, err)
			writeFile(t, p, strings.Replace(string(data), "enforce = true", "enforce = true\nmin_approvers = 1", 1))
		}, "[governance] table"},
		{"nothing relevant changed", func(t *testing.T, root string) {}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := approveProject(t, "approvers_from = \"CODEOWNERS\"\n")
			writeFile(t, filepath.Join(root, "CODEOWNERS"), "* @alice\n")
			crossGit(t, root, "init", "-q", "-b", "main")
			crossGit(t, root, "add", "-A")
			crossGit(t, root, "commit", "-qm", "base")
			tt.change(t, root)
			approveVerifyBase = "main"

			// Act
			code, stdout, stderr := runApproveCmd(t)

			// Assert
			if tt.want == "" {
				assert.Equal(t, 0, code, stderr)
				return
			}
			assert.Equal(t, 2, code, stderr)
			assert.Contains(t, stdout, "AR716")
			assert.Contains(t, stdout, tt.want)
		})
	}
}
