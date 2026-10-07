package config

import (
	"io/fs"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

// A path from a root is rooted on every platform, also where filepath.IsAbs
// says otherwise (Windows: "/virtual/proj" has no volume).
func TestRooted(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/virtual/proj", true},
		{"/etc/passwd", true},
		{`\virtual\proj`, true},
		{"rel/dir", false},
		{"./dir", false},
		{"../up", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.want, rooted(tt.path))
		})
	}
}

// A resource read from disk on Windows has the mode a git snapshot gives it.
func TestResourceMode(t *testing.T) {
	tests := []struct {
		name string
		in   fs.FileMode
		want fs.FileMode
	}{
		{"a plain file", 0o644, 0o644},
		{"a script", 0o755, 0o755},
		{"a writable file on Windows", 0o666, 0o666},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := tt.want
			if runtime.GOOS == "windows" {
				want = 0o644
			}
			assert.Equal(t, want, resourceMode(tt.in))
		})
	}
}
