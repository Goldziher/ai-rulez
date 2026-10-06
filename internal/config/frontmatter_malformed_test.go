package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	metadata, body, malformed := parseFrontmatter(content)

	if metadata != nil {
		t.Errorf("expected nil metadata for unparseable frontmatter, got %+v", metadata)
	}
	if !malformed {
		t.Error("expected malformed=true for unparseable frontmatter, got false")
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

	metadata, body, malformed := parseFrontmatter(content)

	if metadata == nil {
		t.Fatal("expected metadata for valid frontmatter, got nil")
	}
	if malformed {
		t.Error("expected malformed=false for valid frontmatter, got true")
	}
	if strings.Contains(body, "---") || strings.Contains(body, "description:") {
		t.Errorf("valid frontmatter was not stripped from body:\n%q", body)
	}
}

func TestScanSkills_UnclosedFrontmatterIsMalformed(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{"no closing delimiter", "---\nname: x\ndescription: d\n\nBody.\n", true},
		{"only the opening line", "---\nBody\n", true},
		{"closed", "---\nname: x\ndescription: d\n---\n\nBody.\n", false},
		{"no frontmatter", "Body.\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(dir, "x"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "x", "SKILL.md"), []byte(tt.body), 0o644))

			// Act
			skills, err := (&contentScanner{}).skills(dir, nil)

			// Assert
			require.NoError(t, err)
			require.Len(t, skills, 1)
			assert.Equal(t, tt.want, skills[0].MalformedFrontmatter)
		})
	}
}
