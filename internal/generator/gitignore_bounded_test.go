package generator

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitignoreSymlinkedToADeviceNeverHangs(t *testing.T) {
	if _, err := os.Stat("/dev/zero"); err != nil {
		t.Skip("/dev/zero unavailable")
	}
	tests := []struct {
		name string
		run  func(t *testing.T, p *driftProject) error
	}{
		{"generate", func(t *testing.T, p *driftProject) error { return NewGenerator(p.load(t)).Generate("") }},
		{"dry-run", func(t *testing.T, p *driftProject) error {
			_, err := NewGenerator(p.load(t)).DryRun("")
			return err
		}},
		{"clean", func(t *testing.T, p *driftProject) error {
			require.NoError(t, NewGenerator(p.load(t)).Generate(""))
			_, err := NewGenerator(p.load(t)).Clean("", CleanOptions{})
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			p := newDriftProject(t, driftSharedIgnoring)
			p.git(t, "init", "-q")
			if err := os.Symlink("/dev/zero", filepath.Join(p.base, ".gitignore")); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			p.writeFile(t, ".ai-rulez/local/rules/mine.md", "---\npriority: low\n---\n\nPrivate.\n")

			// Act
			done := make(chan error, 1)
			go func() { done <- tt.run(t, p) }()

			// Assert
			select {
			case err := <-done:
				assert.NoError(t, err)
			case <-time.After(20 * time.Second):
				t.Fatal("run did not finish: the symlinked .gitignore was read unbounded")
			}
		})
	}
}

func TestGenerate_SymlinkedGitignoreKeepsPatternsItListsItself(t *testing.T) {
	// Arrange: the link's target already lists a pattern ai-rulez needs, but git
	// does not read a symlinked .gitignore, so the pattern must still reach
	// .git/info/exclude.
	p := newDriftProject(t, driftSharedIgnoring)
	p.git(t, "init", "-q")
	target := filepath.Join(t.TempDir(), "shared-gitignore")
	require.NoError(t, os.WriteFile(target, []byte(".claude/rules/*.local.*\n.ai-rulez/local/\n"), 0o644))
	if err := os.Symlink(target, filepath.Join(p.base, ".gitignore")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	p.writeFile(t, ".ai-rulez/local/rules/mine.md", "---\npriority: low\n---\n\nPrivate.\n")

	// Act
	err := NewGenerator(p.load(t)).Generate("")

	// Assert
	require.NoError(t, err)
	exclude, readErr := os.ReadFile(filepath.Join(p.base, ".git", "info", "exclude"))
	require.NoError(t, readErr)
	assert.Contains(t, string(exclude), ".claude/rules/*.local.*")
	assert.Contains(t, string(exclude), ".ai-rulez/local/")
}

func TestIgnoreLocalInputs_CoversTheOverlayLockAndTempFilesBeforeTheyExist(t *testing.T) {
	// Arrange: a local rule makes the project local, without any lock file on disk.
	p := newDriftProject(t, driftSharedIgnoring)
	p.git(t, "init", "-q")
	p.writeFile(t, ".ai-rulez/local/rules/mine.md", "---\npriority: low\n---\n\nPrivate.\n")

	// Act
	err := NewGenerator(p.load(t)).ignoreLocalInputs()

	// Assert
	require.NoError(t, err)
	assert.Contains(t, p.read(t, ".gitignore"), ".ai-rulez/.config.local.*")
	assert.True(t, p.checkIgnored(t, ".ai-rulez/.config.local.toml.lock"))
}

func TestGenerate_UserNegationOfALocalInputFailsClosed(t *testing.T) {
	tests := []struct {
		name     string
		negation string
		want     string
	}{
		{"local content tree", "!.ai-rulez/local/\n", ".ai-rulez/local"},
		{"overlay file", "!.ai-rulez/config.local.toml\n", ".ai-rulez/config.local.toml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			p := newDriftProject(t, driftSharedIgnoring)
			p.git(t, "init", "-q")
			p.overlay(t, "name = \"renamed\"\n")
			p.writeFile(t, ".ai-rulez/local/rules/mine.md", "---\npriority: low\n---\n\nPrivate.\n")
			require.NoError(t, NewGenerator(p.load(t)).Generate(""))
			// A negation below the managed block beats it.
			p.writeFile(t, ".gitignore", p.read(t, ".gitignore")+tt.negation)

			// Act
			err := NewGenerator(p.load(t)).Generate("")

			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), "not git-ignored")
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}
