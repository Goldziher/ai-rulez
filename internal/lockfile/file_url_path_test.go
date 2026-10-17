package lockfile

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFileURLPath(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
		ok     bool
		onlyOS string
	}{
		{"unix path", "file:///srv/repo", filepath.FromSlash("/srv/repo"), true, ""},
		{"git+file prefix", "git+file:///srv/repo", filepath.FromSlash("/srv/repo"), true, ""},
		{"upper-case scheme", "FILE:///srv/repo", filepath.FromSlash("/srv/repo"), true, ""},
		{"drive letter as host", "file://D:/a/repo", filepath.FromSlash("D:/a/repo"), true, ""},
		{"drive letter in the path", "file:///D:/a/repo", filepath.FromSlash("D:/a/repo"), true, "windows"},
		{"not a file url", "https://example.com/repo", "", false, ""},
		{"scp-like", "git@github.com:org/repo.git", "", false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			if tt.onlyOS != "" && runtime.GOOS != tt.onlyOS {
				t.Skipf("applies to %s only", tt.onlyOS)
			}

			// Act
			got, ok := FileURLPath(tt.source)

			// Assert
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}
