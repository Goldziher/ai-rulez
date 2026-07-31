package config

import (
	"strings"
	"testing"
)

// TestParseFrontmatter_MalformedIsStripped is regression coverage for #156:
// when a delimited frontmatter block is present but its YAML is unparseable
// (e.g. an unquoted value containing ": "), parseFrontmatter must strip the
// block rather than returning the original content verbatim. Returning the
// unstripped content caused a second (raw) frontmatter block to be emitted
// after the generated one in every affected SKILL.md.
func TestParseFrontmatter_MalformedIsStripped(t *testing.T) {
	content := "---\n" +
		"description: OWASP Top 10 quick reference: the ten most critical risks\n" +
		"---\n\n" +
		"Body line one.\nBody line two.\n"

	metadata, body := parseFrontmatter(content)

	if metadata != nil {
		t.Errorf("expected nil metadata for unparseable frontmatter, got %+v", metadata)
	}
	if strings.Contains(body, "---") {
		t.Errorf("frontmatter delimiters were not stripped; body still contains ---:\n%q", body)
	}
	if strings.Contains(body, "description:") {
		t.Errorf("malformed frontmatter leaked into body:\n%q", body)
	}
	if !strings.HasPrefix(body, "Body line one.") {
		t.Errorf("body should start with the real content, got:\n%q", body)
	}
}

// TestParseFrontmatter_ValidStillParses guards against over-stripping: a valid
// frontmatter block (description quoted so the ": " is legal) must parse into
// metadata and have its block stripped from the body.
func TestParseFrontmatter_ValidStillParses(t *testing.T) {
	content := "---\n" +
		"description: \"OWASP Top 10 quick reference: the ten most critical risks\"\n" +
		"---\n\n" +
		"Body line one.\n"

	metadata, body := parseFrontmatter(content)

	if metadata == nil {
		t.Fatal("expected metadata for valid frontmatter, got nil")
	}
	if strings.Contains(body, "---") || strings.Contains(body, "description:") {
		t.Errorf("valid frontmatter was not stripped from body:\n%q", body)
	}
}
