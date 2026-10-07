package llmstxt

import (
	"strings"
	"testing"
)

func codes(fs []Finding) []string {
	out := make([]string, 0, len(fs))
	for _, f := range fs {
		out = append(out, f.Code)
	}
	return out
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want []string
	}{
		{"minimal title only", "# Project\n", nil},
		{"full valid", "# P\n\n> Summary.\n\nSome details.\n\n## Docs\n\n- [A](a.md): note\n- [B](https://x.dev/b)\n\n## Optional\n\n- [C](c.md)\n", nil},
		{"bom accepted", "\ufeff# P\n", nil},
		{"empty file", "", []string{CodeTitleMissing}},
		{"no h1 first", "Hello\n\n# P\n", []string{CodeTitleMissing}},
		{"h2 first", "## Docs\n\n- [A](a.md)\n", []string{CodeTitleMissing}},
		{"second h1", "# P\n\n# Q\n", []string{CodeTitleMissing}},
		{"summary after details", "# P\n\ntext\n\n> late\n", []string{CodeSummaryMisplaced}},
		{"empty summary", "# P\n\n>\n", []string{CodeSummaryMisplaced}},
		{"h3 in details", "# P\n\n### Sub\n", []string{CodeHeadingInvalid}},
		{"h3 in section", "# P\n\n## S\n\n### Sub\n\n- [A](a.md)\n", []string{CodeHeadingInvalid}},
		{"empty h2 name", "# P\n\n##\n\n- [A](a.md)\n", []string{CodeHeadingInvalid}},
		{"item without link", "# P\n\n## S\n\n- just text\n", []string{CodeLinkEntryInvalid}},
		{"paragraph in section", "# P\n\n## S\n\nprose here\n\n- [A](a.md)\n", []string{CodeLinkEntryInvalid}},
		{"ordered list item", "# P\n\n## S\n\n1. [A](a.md)\n", nil},
		{"star bullet", "# P\n\n## S\n\n* [A](a.md): n\n", nil},
		{"optional not last", "# P\n\n## Optional\n\n- [A](a.md)\n\n## S\n\n- [B](b.md)\n", []string{CodeOptionalMisplaced}},
		{"optional duplicated", "# P\n\n## Optional\n\n- [A](a.md)\n\n## Optional\n\n- [B](b.md)\n", []string{CodeOptionalMisplaced, CodeSectionDuplicateOrEmpty}},
		{"duplicate section", "# P\n\n## S\n\n- [A](a.md)\n\n## S\n\n- [B](b.md)\n", []string{CodeSectionDuplicateOrEmpty}},
		{"empty section", "# P\n\n## S\n", []string{CodeSectionDuplicateOrEmpty}},
		{"empty url", "# P\n\n## S\n\n- [A]()\n", []string{CodeLinkTargetInvalid}},
		{"url with space", "# P\n\n## S\n\n- [A](a b.md)\n", []string{CodeLinkEntryInvalid}},
		{"fenced code ignored", "# P\n\n```\n# not a heading\n### nor this\n```\n\n## S\n\n- [A](a.md)\n", nil},
		{"hash in link note", "# P\n\n## S\n\n- [A](a.md): see C# docs\n", nil},
		{"crlf", "# P\r\n\r\n> S\r\n\r\n## D\r\n\r\n- [A](a.md)\r\n", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := codes(Validate([]byte(tc.src)))
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("codes = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestValidateFindingMetadata(t *testing.T) {
	fs := Validate([]byte("# P\n\n## S\n\n- nope\n"))
	if len(fs) != 1 {
		t.Fatalf("findings = %v", fs)
	}
	f := fs[0]
	if f.Line != 5 || f.Name != "llmstxt-link-entry-invalid" || f.Severity != SeverityError || f.Message == "" {
		t.Fatalf("finding = %+v", f)
	}
}

func TestRulesComplete(t *testing.T) {
	seen := map[string]bool{}
	for _, r := range Rules() {
		if seen[r.Code] || r.Name == "" || r.Describe == "" {
			t.Fatalf("bad rule %+v", r)
		}
		seen[r.Code] = true
	}
	for _, c := range []string{CodeTitleMissing, CodeSummaryMisplaced, CodeHeadingInvalid, CodeLinkEntryInvalid, CodeOptionalMisplaced, CodeSectionDuplicateOrEmpty, CodeLinkTargetInvalid} {
		if !seen[c] {
			t.Fatalf("code %s not in Rules()", c)
		}
	}
}
