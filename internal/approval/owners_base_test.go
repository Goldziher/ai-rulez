package approval

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

const ownershipBase = "version = \"5.0\"\nname = \"solo\"\npresets = [\"claude\"]\n\n[governance]\nrequire_approval = [\"remote\"]\nmin_approvers = 1\n"

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{"-C", dir, "-c", "user.email=t@example.com", "-c", "user.name=t", "-c", "commit.gpgsign=false"}, args...)
	res := gitutil.New(nil).Exec(context.Background(), "", nil, full...)
	require.Equal(t, runner.StatusOK, res.Status, string(res.Stderr))
	return string(res.Stdout)
}

func writeOwnersFile(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func TestOwnershipChanges_ReadsTheGovernanceTableOfEachLayout(t *testing.T) {
	tests := []struct {
		name     string
		file     string // the config file, relative to the project
		edited   string
		wantRel  string
		wantNone bool
	}{
		{"root layout, table edited", "ai-rulez.toml", ownershipBase + "approvers = [\"mallory\"]\n", "ai-rulez.toml", false},
		{"directory layout, table edited", ".ai-rulez/config.toml", ownershipBase + "approvers = [\"mallory\"]\n", ".ai-rulez/config.toml", false},
		{"root layout, another table edited", "ai-rulez.toml", ownershipBase + "\n[lint]\nfail_on = \"warning\"\n", "", true},
		{"root layout, nothing edited", "ai-rulez.toml", ownershipBase, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			if real, err := filepath.EvalSymlinks(root); err == nil {
				root = real
			}
			path := filepath.Join(root, filepath.FromSlash(tt.file))
			writeOwnersFile(t, path, ownershipBase)
			gitIn(t, root, "init", "-q")
			gitIn(t, root, "add", "-A")
			gitIn(t, root, "commit", "-q", "-m", "base")
			base := gitIn(t, root, "rev-parse", "HEAD")
			writeOwnersFile(t, path, tt.edited)
			cfg, err := config.LoadConfig(context.Background(), root, config.WithoutRemote())
			require.NoError(t, err)

			// Act
			got, err := OwnershipChanges(context.Background(), cfg, trimNewline(base))

			// Assert
			require.NoError(t, err)
			if tt.wantNone {
				assert.Empty(t, got)
				return
			}
			require.Len(t, got, 1)
			assert.Contains(t, got[0], "the [governance] table of "+tt.wantRel+" changed since ")
		})
	}
}

func TestOwnershipChanges_RefusesAProjectOutsideAGitWorkTree(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeOwnersFile(t, filepath.Join(root, "ai-rulez.toml"), ownershipBase)
	cfg, err := config.LoadConfig(context.Background(), root, config.WithoutRemote())
	require.NoError(t, err)

	// Act
	_, err = OwnershipChanges(context.Background(), cfg, "HEAD")

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not inside a git work tree")
}

func trimNewline(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
