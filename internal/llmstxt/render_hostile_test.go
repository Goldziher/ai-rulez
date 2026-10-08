package llmstxt

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRenderNeutralisesHostileLinks(t *testing.T) {
	hostile := []string{
		"a\n\n## injected\n\nb.md",
		"a\r\n# injected\r\n.md",
		"a<script>x</script>.md",
		"a>b<c.md",
		"a](https://evil.example)[b.md",
		"x\n- [y](https://evil.example)",
		"tab\there (paren).md",
	}
	for _, h := range hostile {
		t.Run(h, func(t *testing.T) {
			// Arrange
			d := Doc{Title: "T", Sections: []Section{{Name: "Rules", Links: []Link{
				{Title: h, URL: h, Note: "see [click](https://evil.example) ![img](https://evil.example/i.png) <b>" + h},
			}}}}

			// Act
			out := d.Render()

			// Assert
			lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
			assert.Len(t, lines, 5, out) // "# T", "", "## Rules", "", one list item
			assert.True(t, strings.HasPrefix(lines[4], "- ["), out)
			assert.NotContains(t, out, "\r")
			assert.NotRegexp(t, `(^|[^\\])[<>]`, lines[4], "unescaped angle bracket")
			assert.NotRegexp(t, `(^|[^\\])\[click\]`, lines[4], "live note link")
			assert.NotContains(t, strings.ReplaceAll(lines[4], `\[`, ""), "[click]")
			assert.Len(t, strings.Fields(linkTargets(lines[4])), 1, "exactly the one intended link: %q", lines[4])
		})
	}
}

// linkTargets returns the markdown link targets live in a line.
func linkTargets(line string) string {
	var sb strings.Builder
	for _, m := range linkRe.FindAllStringSubmatch(line, -1) {
		sb.WriteString(m[1] + "\n")
	}
	return sb.String()
}

func TestRenderFullDemotesSetextHeadings(t *testing.T) {
	// Arrange
	body := "Title\n=====\n\ntext\n\nSub\n---\n\n- item\n---\n\n```\nCode\n----\n```\n"

	// Act
	out := RenderFull("T", "", []Page{{Title: "P", Body: body}})

	// Assert
	assert.Contains(t, out, "\n### Title\n")
	assert.Contains(t, out, "\n#### Sub\n")
	assert.NotContains(t, out, "=====")
	assert.Contains(t, out, "Code\n----\n", "fenced code is left alone")
	for _, line := range strings.Split(out, "\n") {
		if m := headingRe.FindStringSubmatch(line); m != nil && line != "# T" && line != "## P" {
			assert.GreaterOrEqual(t, len(m[1]), 2, line)
		}
	}
}
