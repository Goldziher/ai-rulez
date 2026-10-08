package llmstxt

import (
	"slices"
	"strings"
	"testing"
)

func FuzzValidate(f *testing.F) {
	for _, seed := range []string{
		"# Title\n\n> Summary.\n\n## Docs\n\n- [A](https://example.com/a): note\n",
		"# Title\n\n## Optional\n\n- [B](b.md)\n",
		"", "#", "# \n", "\ufeff# T\r\n\r\n> s\r\n", "```\n# not a heading\n```\n", "# T\n\n- [x](\n", "## No title\n\n- [a](b \"t\")\n",
		"# T\n~~~\n## in fence\n~~~\n", "1. [n](u)\n", strings.Repeat("#", 7) + " seven\n",
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, src []byte) {
		// Act
		got := Validate(src)
		again := Validate(src)

		// Assert: deterministic, ordered by line then code, lines inside the text
		if !slices.Equal(got, again) {
			t.Fatalf("Validate is not deterministic for %q", src)
		}
		lines := strings.Count(string(src), "\n") + 1
		for i, fd := range got {
			if fd.Line < 0 || fd.Line > lines {
				t.Fatalf("finding %+v is outside the %d lines of %q", fd, lines, src)
			}
			if i > 0 && (got[i-1].Line > fd.Line || (got[i-1].Line == fd.Line && got[i-1].Code > fd.Code)) {
				t.Fatalf("findings are not ordered by line and code: %+v then %+v", got[i-1], fd)
			}
		}
	})
}
