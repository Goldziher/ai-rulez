package conformance

import (
	"os"
	"path/filepath"
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

// TestPublishedDocsSiteLLMSTxtFollowsTheFormat checks the llms.txt and llms-full.txt of the docs site,
// which the site build writes to website/dist (`pnpm run build` in website/). The docs workflow runs
// it after the build; without a build there is nothing to check.
func TestPublishedDocsSiteLLMSTxtFollowsTheFormat(t *testing.T) {
	dist := filepath.Join(repoRoot, "website", "dist")
	if _, err := os.Stat(filepath.Join(dist, "llms.txt")); err != nil {
		// The docs workflow sets DOCS_SITE_REQUIRED so a moved build output fails instead of skipping.
		require.Empty(t, os.Getenv("DOCS_SITE_REQUIRED"), "the docs site is not built at %s", dist)
		t.Skip("the docs site is not built; run `pnpm run build` in website/ first")
	}
	index := string(repoFile(t, "website/dist/llms.txt"))
	checkLLMSIndex(t, index)
	checkLLMSFindings(t, index)
	// llms-full.txt has no fixed format: the site writes a preamble line, then every page.
	full := string(repoFile(t, "website/dist/llms-full.txt"))
	assert.Regexp(t, `(?m)^# \S`, full, "llms-full.txt holds the pages' Markdown")
	assert.True(t, utf8.ValidString(full))
}

func TestLLMSTxtCheckerRejectsMalformedIndexes(t *testing.T) {
	assert.NotRegexp(t, llmsH1, "no title")
	assert.NotRegexp(t, llmsLink, "- not a link")
	assert.Regexp(t, llmsLink, "- [Doc](https://example.com/doc): about it")
}
