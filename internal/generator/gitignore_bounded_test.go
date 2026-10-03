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
