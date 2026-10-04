package lint

import "testing"

func TestGlobMatch(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"**/*.py", "a/b/c.py", true},
		{"**/*.py", "c.py", true},
		{"*.py", "deep/dir/c.py", true},
		{"src/*.ts", "src/a.ts", true},
		{"src/*.ts", "src/x/a.ts", false},
		{"src/**", "src/x/y/a.ts", true},
		{"src/", "src/a.ts", true},
		{"src", "src/a.ts", true},
		{"src/**/*.{ts,tsx}", "src/a/b.tsx", true},
		{"src/**/*.{ts,tsx}", "src/a/b.js", false},
		{"./docs/*.md", "docs/a.md", true},
		{"/docs/*.md", "docs/a.md", true},
		{"a?c.txt", "abc.txt", true},
		{"a?c.txt", "a/c.txt", false},
		{"[ab].txt", "a.txt", true},
		{"[!ab].txt", "a.txt", false},
		{"apps/**/BUILD", "apps/x/y/BUILD", true},
		{"apps/**/BUILD", "apps/BUILD", true},
		{"nothing/**", "src/a.ts", false},
		{"**/.ai-rulez/**", "pkg/.ai-rulez/config.toml", true},
	}
	for _, tt := range tests {
		t.Run(tt.pattern+"|"+tt.path, func(t *testing.T) {
			g, ok := newGlob(tt.pattern)
			if !ok {
				t.Fatalf("pattern %q did not compile", tt.pattern)
			}
			if got := g.match(tt.path); got != tt.want {
				t.Errorf("match(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
			}
		})
	}
}

func TestExpandBraces(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{{"a", 1}, {"a.{x,y}", 2}, {"{a,b}/{c,d}", 4}, {"{a,{b,c}}", 3}, {"{unclosed", 1}}
	for _, tt := range tests {
		if got := len(expandBraces(tt.in)); got != tt.want {
			t.Errorf("expandBraces(%q) = %d alternatives, want %d", tt.in, got, tt.want)
		}
	}
}
