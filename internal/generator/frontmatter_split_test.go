package generator

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The generator reads the frontmatter of the files it writes and of the rule
// files it classifies by the rules of internal/frontmatter.
func TestFrontmatterEndFollowsTheSharedSplitRules(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		wantEnd  int
		wantOpen bool
	}{
		{"LF", "---\na: 1\n---\nbody\n", len("---\na: 1\n---\n"), true},
		{"CRLF", "---\r\na: 1\r\n---\r\nbody", len("---\r\na: 1\r\n---\r\n"), true},
		{"closing fence at the end of the text", "---\na: 1\n---", len("---\na: 1\n---"), true},
		{"trailing spaces on the closing fence", "---\na: 1\n--- \nbody", len("---\na: 1\n--- \n"), true},
		{"a BOM is skipped but counted in the offset", "\xef\xbb\xbf---\na: 1\n---\nbody", len("\xef\xbb\xbf---\na: 1\n---\n"), true},
		{"unclosed", "---\na: 1\nbody\n", 0, true},
		{"not frontmatter", "# Title\n---\n", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			end, open := frontmatterEnd(tt.in)
			assert.Equal(t, tt.wantEnd, end)
			assert.Equal(t, tt.wantOpen, open)
		})
	}
}

func TestRuleFileFrontmatterRequiresTheBlockAtTheStart(t *testing.T) {
	fields, ok := ruleFileFrontmatter("---\nglobs: \"**/*.go\"\n---\nbody\n")
	assert.True(t, ok)
	assert.Equal(t, "**/*.go", fields["globs"])

	fields, ok = ruleFileFrontmatter("---\r\nalwaysApply: true\r\n---\r\nbody\r\n")
	assert.True(t, ok, "CRLF")
	assert.Equal(t, true, fields["alwaysApply"])

	_, ok = ruleFileFrontmatter("\n---\nglobs: x\n---\nbody\n")
	assert.False(t, ok, "a blank line before the opening fence: the harness does not read it as frontmatter")

	_, ok = ruleFileFrontmatter("---\nglobs: x\n---foo\nbody\n")
	assert.False(t, ok, "---foo is not a closing fence")

	fields, ok = ruleFileFrontmatter("---\n---\nbody\n")
	assert.True(t, ok, "an empty block")
	assert.Empty(t, fields)
}

func TestListingFrontmatterFollowsTheSharedSplitRules(t *testing.T) {
	fields, body := listingFrontmatter("\xef\xbb\xbf---\r\nname: x\r\n---\r\nBody\r\n")
	assert.Equal(t, "x", fields["name"])
	assert.Equal(t, "Body\r\n", body)

	text := "---\nname: x\nBody\n"
	fields, body = listingFrontmatter(text)
	assert.Nil(t, fields, "unclosed")
	assert.Equal(t, text, body)
}

func TestInjectHashIntoAnEmptyFrontmatterBlock(t *testing.T) {
	block := hashLines("# ", "blake3:c", "blake3:s")

	got := injectHashes("---\n---\nbody\n", ".claude/skills/x/SKILL.md", "blake3:c", "blake3:s")

	assert.Equal(t, "---\n"+block+"\n---\nbody\n", got)
	assert.Equal(t, "---\na: 1\n"+block+"\n---\nbody\n", injectHashes("---\na: 1\n---\nbody\n", ".claude/skills/x/SKILL.md", "blake3:c", "blake3:s"))
}
