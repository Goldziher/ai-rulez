package pathglob

import "testing"

func TestMatcherPreservesLiteralPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		path    string
		want    bool
	}{
		{"Unicode directory", "référence/*.md", "référence/guide.md", true},
		{"Unicode filename at any depth", "東京.md", "docs/東京.md", true},
		{"distinct Unicode literal", "café.md", "docs/cafe.md", false},
		{"Unicode beside recursive wildcard", "**/資料/*.md", "project/docs/資料/概要.md", true},
		{"Unicode beside character class", "docs/文[甲乙].md", "docs/文乙.md", true},
		{"single wildcard matches a Unicode rune", "docs/?.md", "docs/猫.md", true},
		{"single star stays in its directory", "docs/*.md", "docs/sub/guide.md", false},
		{"ASCII regex metacharacters stay literal", "docs/notes(1)+.md", "docs/notes(1)+.md", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			matcher, ok := New(tt.pattern)
			if !ok {
				t.Fatalf("New(%q) rejected a valid pattern", tt.pattern)
			}
			if got := matcher.Match(tt.path); got != tt.want {
				t.Errorf("Match(%q) for pattern %q = %t, want %t", tt.path, tt.pattern, got, tt.want)
			}
		})
	}
}

func TestBraceExpansionUsesWholeResultBudget(t *testing.T) {
	t.Parallel()

	remaining := 2
	got := expandBraces("docs/{guide,faq}.md", &remaining, 0)
	if len(got) != 2 || got[0] != "docs/guide.md" || got[1] != "docs/faq.md" || remaining != 0 {
		t.Fatalf("got %v and %d slots left, want both alternatives and no slots", got, remaining)
	}

	remaining = 1
	if got := expandBraces("docs/{guide,faq}.md", &remaining, 0); got != nil {
		t.Fatalf("got partial result %v, want no matches", got)
	}
}
