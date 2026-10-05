package includes

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// A user-controlled repository URL that looks like a git option must reach git
// as an operand, never as an option such as --upload-pack, which runs a command.
func TestGitOpsTreatUserURLAsOperand(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	tests := []struct {
		name string
		run  func(ctx context.Context, url, dest string) error
	}{
		{"ls-remote", func(ctx context.Context, url, _ string) error {
			_, err := remoteHEADSHA(ctx, url, "main", "")
			return err
		}},
		{"sparse clone", func(ctx context.Context, url, dest string) error {
			return sparseClone(ctx, url, "", "skills/x/", dest, "")
		}},
		{"sparse clone sha", func(ctx context.Context, url, dest string) error {
			return sparseCloneSHA(ctx, url, "0123456789abcdef0123456789abcdef01234567", "skills/x/", dest, "")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			marker := filepath.Join(dir, "pwned")
			url := "--upload-pack=touch " + marker
			dest := filepath.Join(dir, "dest")
			if err := os.MkdirAll(dest, 0o755); err != nil {
				t.Fatal(err)
			}

			// Act
			err := tt.run(context.Background(), url, dest)

			// Assert
			if _, statErr := os.Stat(marker); statErr == nil {
				t.Fatalf("git ran the option embedded in the URL")
			}
			if err == nil {
				t.Fatalf("expected an error for a bogus repository")
			}
		})
	}
}
