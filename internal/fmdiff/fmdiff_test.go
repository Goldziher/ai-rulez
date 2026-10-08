// Package fmdiff differential-fuzzes the frontmatter splitters that live
// unexported in several packages. They are reached through go:linkname so no
// production code changes for a test; the empty .s file lets the compiler accept
// the bodyless declarations. A divergence the fuzzer finds is either listed in
// knownDivergence (documented, not fixed here) or fails the run.
package fmdiff

import (
	"slices"
	"strings"
	"testing"
	_ "unsafe" // go:linkname

	"github.com/Goldziher/ai-rulez/v5/internal/okf"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"

	_ "github.com/Goldziher/ai-rulez/v5/internal/crud"
	_ "github.com/Goldziher/ai-rulez/v5/internal/importer"
	_ "github.com/Goldziher/ai-rulez/v5/internal/improve"
	_ "github.com/Goldziher/ai-rulez/v5/internal/llmstxt"
	_ "github.com/Goldziher/ai-rulez/v5/internal/migrate"
	_ "github.com/Goldziher/ai-rulez/v5/internal/review"
)

//go:linkname importerSplit github.com/Goldziher/ai-rulez/v5/internal/importer.splitFrontmatter
func importerSplit(content string) (fm, body string, has bool)

//go:linkname crudSplit github.com/Goldziher/ai-rulez/v5/internal/crud.splitFrontmatter
func crudSplit(content string) (fm, body string, has bool)

//go:linkname migrateSplit github.com/Goldziher/ai-rulez/v5/internal/migrate.splitFrontmatter
func migrateSplit(text string) (fm, rest string, ok bool)

//go:linkname reviewSplit github.com/Goldziher/ai-rulez/v5/internal/review.splitFrontmatter
func reviewSplit(raw string) (meta map[string]any, body string)

//go:linkname improveSplit github.com/Goldziher/ai-rulez/v5/internal/improve.splitFrontmatter
func improveSplit(data []byte) (fm map[string]any, body string, ok bool)

//go:linkname llmstxtSplit github.com/Goldziher/ai-rulez/v5/internal/llmstxt.splitFrontmatter
func llmstxtSplit(src string) (meta map[string]string, body string)

type split struct {
	has  bool
	body string
}

var splitters = []struct {
	name string
	run  func(string) split
}{
	{"importer", func(s string) split { _, b, h := importerSplit(s); return split{h, b} }},
	{"crud", func(s string) split { _, b, h := crudSplit(s); return split{h, b} }},
	{"migrate", func(s string) split { _, b, h := migrateSplit(s); return split{h, b} }},
	{"okf", func(s string) split {
		fm, b := okf.SplitFrontmatter([]byte(s))
		return split{fm.Present && (fm.Err == nil || !strings.Contains(fm.Err.Error(), "not closed")), b}
	}},
	{"review", func(s string) split { _, b := reviewSplit(s); return split{b != s, b} }},
	{"improve", func(s string) split { _, b, h := improveSplit([]byte(s)); return split{h, b} }},
	{"llmstxt", func(s string) split { _, b := llmstxtSplit(s); return split{b != s, b} }},
}

// reference is the contract the splitters share on LF-only input: the block
// closes at the first line that is exactly "---".
func reference(data string) bool {
	return slices.Contains(strings.Split(data, "\n"), "---")
}

// block is the text before the first line that is exactly "---".
func block(data string) string {
	var out []string
	for _, line := range strings.Split(data, "\n") {
		if line == "---" {
			break
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// knownDivergence names the documented reason a splitter disagrees with the
// reference on input in ("---\n" + data), or "" when the disagreement is new.
func knownDivergence(name, data string, got, want bool) string {
	closing := false
	for _, line := range strings.Split(data, "\n") {
		if line == "---" {
			break
		}
		if strings.HasPrefix(line, "---") {
			closing = true // a line that starts with --- but is not exactly ---
			break
		}
	}
	switch {
	case got && !want && closing && (name == "migrate" || name == "improve" || name == "review"):
		return "closing fence is any line starting with ---, not only an exact --- line"
	case !got && want && (data == "---" || strings.HasPrefix(data, "---\n")) && (name == "migrate" || name == "improve" || name == "review" || name == "llmstxt"):
		return "an empty block (--- directly followed by ---) is not recognised"
	case !got && want && name == "llmstxt" && !strings.Contains(data, "\n---\n"):
		return "llmstxt needs a newline after the closing fence, so a fence at end of file is missed"
	}
	return ""
}

func FuzzFrontmatterSplittersAgree(f *testing.F) {
	for _, seed := range []string{
		"k: v\n---\nbody", "---\nbody", "---", "k: v\n---", "k: v\n---x\n", "k: v\n--- \n", "a:\n  - b\n---\n\n# t\n",
		"k: v\nno close", "", "\n\n---\n---\n", "# comment\n---\n",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data string) {
		// CR and BOM handling differ by design (see TestBOMAndCarriageReturnHandling).
		if strings.ContainsAny(data, "\r\ufeff\x00") {
			t.Skip()
		}
		var m map[string]any
		if err := yaml.Unmarshal([]byte(block(data)), &m); err != nil {
			t.Skip() // each splitter treats invalid YAML its own way
		}
		if m == nil && strings.TrimSpace(block(data)) != "" {
			t.Skip() // a block that decodes to null (an anchor alone, a bare tag): review reads it as no frontmatter
		}
		in := "---\n" + data
		want := reference(data)
		for _, s := range splitters {
			if got := s.run(in).has; got != want && knownDivergence(s.name, data, got, want) == "" {
				t.Errorf("%s: frontmatter=%v, the reference says %v for %q", s.name, got, want, in)
			}
		}
	})
}

// The splitters differ on BOM and carriage returns by design of their callers;
// this pins each so a change is deliberate.
func TestBOMAndCarriageReturnHandling(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  map[string]bool // frontmatter found, per splitter
	}{
		{"bom", "\ufeff---\nk: v\n---\nb", map[string]bool{"importer": true, "crud": false, "migrate": false, "okf": true, "review": true, "improve": false, "llmstxt": false}},
		{"crlf", "---\r\nk: v\r\n---\r\nb", map[string]bool{"importer": true, "crud": true, "migrate": false, "okf": true, "review": true, "improve": true, "llmstxt": true}},
		{"lone cr", "---\rk: v\r---\rb", map[string]bool{"importer": true, "crud": false, "migrate": false, "okf": false, "review": false, "improve": false, "llmstxt": false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, s := range splitters {
				assert.Equal(t, tt.want[s.name], s.run(tt.input).has, s.name)
			}
		})
	}
}
