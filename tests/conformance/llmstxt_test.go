package conformance

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/llmstxt"
)

var (
	llmsLink = regexp.MustCompile(`^[-*] \[[^\]]+\]\([^)\s]+\)(?:: .+)?$`)
	llmsH1   = regexp.MustCompile(`^# \S`)
)

// checkLLMSIndex checks src against the llms.txt format (https://llmstxt.org/#format),
// written out here independently of internal/llmstxt: an H1 with the name as the
// first line (the only required part), optionally followed by a blockquote,
// free text, then H2 sections that are lists of `- [name](url): notes` links.
func checkLLMSIndex(t *testing.T, src string) {
	t.Helper()
	require.True(t, utf8.ValidString(src))
	lines := strings.Split(strings.TrimSuffix(src, "\n"), "\n")
	require.NotEmpty(t, lines)
	assert.Regexp(t, llmsH1, lines[0], "the first line is the H1 title")
	section := ""
	for i, line := range lines[1:] {
		no := i + 2
		switch {
		case strings.HasPrefix(line, "# "):
			t.Errorf("line %d: a second H1", no)
		case strings.HasPrefix(line, "## "):
			section = strings.TrimSpace(line[3:])
			assert.NotEmpty(t, section, "line %d", no)
		case strings.HasPrefix(line, "#"):
			t.Errorf("line %d: only the H1 and H2 headings are part of the format", no)
		case section != "" && strings.TrimSpace(line) != "":
			assert.Regexp(t, llmsLink, line, "line %d: a section holds a list of links", no)
		}
	}
}

func checkLLMSFindings(t *testing.T, src string) {
	t.Helper()
	for _, f := range llmstxt.Validate([]byte(src)) {
		assert.NotEqual(t, llmstxt.SeverityError, f.Severity, "%s line %d: %s", f.Code, f.Line, f.Message)
	}
}

func TestGeneratedLLMSTxtFollowsTheFormat(t *testing.T) {
	// Arrange
	dir := project(t, map[string]string{
		".ai-rulez/config.toml":            "version = \"5.0\"\nname = \"conf\"\npresets = [\"llms-txt\"]\n\n[llms_txt]\nfull = true\n",
		".ai-rulez/rules/r.md":             "# Rule\n\nBe nice.\n",
		".ai-rulez/skills/deploy/SKILL.md": "---\nname: deploy\ndescription: Ship it.\n---\nDeploy.\n",
	})

	// Act
	run(t, dir, "generate", "--yes")

	// Assert
	index := string(read(t, dir, "llms.txt"))
	checkLLMSIndex(t, index)
	checkLLMSFindings(t, index)
	full := string(read(t, dir, "llms-full.txt"))
	assert.Regexp(t, llmsH1, strings.SplitN(full, "\n", 2)[0])
	assert.True(t, utf8.ValidString(full))
}

func TestPublishedDocsSiteLLMSTxtFollowsTheFormat(t *testing.T) {
	index := string(repoFile(t, "docs/llms.txt"))
	checkLLMSIndex(t, index)
	checkLLMSFindings(t, index)
	full := string(repoFile(t, "docs/llms-full.txt"))
	assert.Regexp(t, llmsH1, strings.SplitN(full, "\n", 2)[0])
}

func TestLLMSTxtCheckerRejectsMalformedIndexes(t *testing.T) {
	assert.NotRegexp(t, llmsH1, "no title")
	assert.NotRegexp(t, llmsLink, "- not a link")
	assert.Regexp(t, llmsLink, "- [Doc](https://example.com/doc): about it")
}
