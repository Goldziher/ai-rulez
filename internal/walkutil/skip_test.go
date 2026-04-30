package walkutil

import "testing"

func TestShouldSkipDir(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		skip bool
	}{
		{"", false},
		{".", false},
		{"src", false},
		{"target", true},
		{"node_modules", true},
		{".venv", true},
		{".git", true},
		{".cache", true},
		{".ai-rulez", false},
		{".github", false},
		{".cargo", true},
		{".rustup", true},
		{"vendor", true},
		{"dist", true},
		{"build", true},
		{"__pycache__", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ShouldSkipDir(tc.name); got != tc.skip {
				t.Errorf("ShouldSkipDir(%q) = %v, want %v", tc.name, got, tc.skip)
			}
		})
	}
}
