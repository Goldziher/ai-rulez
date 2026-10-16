package commands

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/render"
)

func TestInitYesNeverPromptsToReplaceAnExistingDirectory(t *testing.T) {
	tests := []struct {
		name   string
		yes    bool
		answer bool
		want   bool
		asked  bool
	}{
		{"--yes does not ask and does not replace", true, true, false, false},
		{"without --yes the prompt decides: yes", false, true, true, true},
		{"without --yes the prompt decides: no", false, false, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			asked := false
			ask := func(string) bool { asked = true; return tt.answer }

			// Act
			got := confirmReplace(".ai-rulez/", tt.yes, ask)

			// Assert
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.asked, asked)
		})
	}
}

func TestBackupOfAConfigDirIsIgnoredByGit(t *testing.T) {
	// Arrange
	root := t.TempDir()
	t.Chdir(root)
	gitIn(t, root, "init", "-q")
	require.NoError(t, os.MkdirAll(".ai-rulez", 0o755))
	require.NoError(t, os.WriteFile(".ai-rulez/config.toml", []byte("x\n"), 0o644))

	// Act
	backup, err := backupConfigDir(".ai-rulez", time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))

	// Assert
	require.NoError(t, err)
	out, err := gitutil.CommandNoContext(root, "status", "--porcelain", "--untracked-files=all").CombinedOutput()
	require.NoError(t, err, string(out))
	assert.NotContains(t, string(out), backup, "the backup directory is untracked and not ignored")
}

func TestInitAnnouncementIsHumanOnly(t *testing.T) {
	tests := []struct {
		name   string
		format string
		want   bool
	}{
		{"text prints the summary", "text", true},
		{"json prints no summary", "json", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			var stdout, stderr bytes.Buffer
			out := render.New(&stdout, &stderr, false).WithFormat(tt.format)

			// Act
			announceInit(out, "proj", ".ai-rulez")

			// Assert
			assert.Equal(t, tt.want, strings.Contains(stdout.String()+stderr.String(), "Created .ai-rulez/ for proj"))
		})
	}
}
