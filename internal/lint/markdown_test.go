package lint

import (
	"reflect"
	"testing"
)

func TestSlugify(t *testing.T) {
	tests := map[string]string{
		"Hello World":         "hello-world",
		"What's `new`?":       "whats-new",
		"API v2.0 (beta)":     "api-v20-beta",
		"snake_case & dashes": "snake_case--dashes",
	}
	for in, want := range tests {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHeadingSlugsDuplicatesAndFences(t *testing.T) {
	raw := "---\ntitle: x\n---\n# Intro\n## Setup\n## Setup\n```\n# not a heading\n```\n"
	got := headingSlugs(raw)
	for _, want := range []string{"intro", "setup", "setup-1"} {
		if _, ok := got[want]; !ok {
			t.Errorf("missing slug %q in %v", want, got)
		}
	}
	if _, ok := got["not-a-heading"]; ok {
		t.Error("heading inside a code fence must not produce a slug")
	}
}

func TestLinkTargets(t *testing.T) {
	tests := []struct {
		name string
		line string
		want []string
	}{
		{"inline", "see [a](docs/a.md) and [b](<b c.md#x> \"t\")", []string{"docs/a.md", "b c.md#x"}},
		{"image", "![logo](img/l.png)", []string{"img/l.png"}},
		{"reference", "[ref]: ./x.md", []string{"./x.md"}},
		{"none", "plain text", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := linkTargets(tt.line); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("linkTargets = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBodySkipsFrontmatterFencesAndBlanksCode(t *testing.T) {
	d := parseDoc("---\na: 1\n---\nline [x](y.md) `[c](z.md)`\n```\n[k](k.md)\n```\nlast")
	body := d.body()
	if len(body) != 2 || body[0].No != 4 || body[1].No != 8 {
		t.Fatalf("unexpected body lines: %+v", body)
	}
	if got := linkTargets(body[0].Plain); !reflect.DeepEqual(got, []string{"y.md"}) {
		t.Errorf("inline-code link must be ignored, got %v", got)
	}
}

func TestIgnoreDirective(t *testing.T) {
	tests := []struct {
		line  string
		codes []string
		found bool
	}{
		{"text <!-- ai-rulez-lint-ignore -->", []string{}, true},
		{"<!-- ai-rulez-lint-ignore: AR401, AR301 -->", []string{"AR401", "AR301"}, true},
		{"nothing here", nil, false},
	}
	for _, tt := range tests {
		codes, found := ignoreDirective(tt.line)
		if found != tt.found || !reflect.DeepEqual(codes, tt.codes) {
			t.Errorf("ignoreDirective(%q) = %v,%v want %v,%v", tt.line, codes, found, tt.codes, tt.found)
		}
	}
}
