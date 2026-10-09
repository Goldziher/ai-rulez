package generator

import (
	"path/filepath"
	"testing"
)

func TestUnderAnyIsAPathContainmentCheck(t *testing.T) {
	root := filepath.Join(t.TempDir(), "home")
	tests := []struct {
		name string
		path string
		want bool
	}{
		{"the root itself", root, true},
		{"a child", filepath.Join(root, "a", "b"), true},
		{"a sibling sharing the prefix", root + "x", false},
		{"the parent", filepath.Dir(root), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := underAny(tt.path, []string{root}); got != tt.want {
				t.Fatalf("underAny(%q, %q) = %v, want %v", tt.path, root, got, tt.want)
			}
		})
	}
}
