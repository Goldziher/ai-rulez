package handlers

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// uncoveredIncludeProject is a project with a remote (file://) include and, when
// lockFile is set, an ai-rulez.lock that does not cover it.
func uncoveredIncludeProject(t *testing.T, lockFile bool) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	repo := t.TempDir()
	rule := filepath.Join(repo, ".ai-rulez", "rules", "shared.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(rule), 0o755))
	require.NoError(t, os.WriteFile(rule, []byte("# Shared\n\nBe kind.\n"), 0o600))
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"}, {"commit", "-qm", "one"}} {
		cmd := exec.Command("git", append([]string{"-c", "user.email=t@example.com", "-c", "user.name=t", "-c", "commit.gpgsign=false"}, args...)...)
		cmd.Dir = repo
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(cfgDir, 0o755))
	cfg := generateSharedConfig + "\n[[includes]]\nname = \"shared\"\nsource = \"file://" + filepath.ToSlash(repo) + "\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(cfg), 0o600))
	if lockFile {
		require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "ai-rulez.lock"), []byte("version = 1\n"), 0o600))
	}
	return dir
}

// TestGenerateOutputsHandler_EnforcedLockRefusesAnUncoveredInclude is
// RV-ENGINE-1 for MCP: generate_outputs writes outputs, so it must refuse a
// remote include an enforced ai-rulez.lock does not cover, as `generate` does.
func TestGenerateOutputsHandler_EnforcedLockRefusesAnUncoveredInclude(t *testing.T) {
	tests := []struct {
		name      string
		lockFile  bool
		wantError bool
	}{
		{name: "an enforced lock that does not cover the include refuses", lockFile: true, wantError: true},
		{name: "without a lock the include is fetched", lockFile: false, wantError: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := uncoveredIncludeProject(t, tt.lockFile)
			args := map[string]any{"working_directory": dir, "no_local": true}

			// Act
			res, err := GenerateOutputsHandler(context.Background(), newRequestWithArgs(args))

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.wantError, res.IsError, textOf(t, res))
			_, statErr := os.Stat(filepath.Join(dir, "CLAUDE.md"))
			assert.Equal(t, !tt.wantError, statErr == nil, "outputs are written only when the load is accepted")
			if tt.wantError {
				assert.Contains(t, textOf(t, res), "not covered by ai-rulez.lock")
			}
		})
	}
}
