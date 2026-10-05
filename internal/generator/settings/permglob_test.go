package settings

import "testing"

func TestGlobsOverlap(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"git *", "git status", true},
		{"git status", "git *", true},
		{"git *", "git", true}, // a trailing " *" also matches the bare command
		{"git status", "git push", false},
		{"*", "anything", true},
		{"npm run *", "yarn *", false},
		{"a?c", "abc", true},
		{"a?c", "axd", false},
		{"*.env", ".env*", true},
		{"src/**", "docs/**", false},
		{"", "", true},
		{"x*", "", false},
		{"*", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.a+"|"+tt.b, func(t *testing.T) {
			if got := globsOverlap(tt.a, tt.b); got != tt.want {
				t.Errorf("globsOverlap(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
