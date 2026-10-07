package catalogsite

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/govview"
)

// renderExcerpt renders one item whose excerpt is body, with Markdown on, and
// returns the item page after checking it against the HTML allow-list.
func renderExcerpt(t *testing.T, body string) string {
	t.Helper()
	doc := manyItemsDoc(1)
	doc.Items[0].Excerpt = &govview.Excerpt{Text: body}
	site, err := Render(doc, Options{Markdown: true})
	require.NoError(t, err)
	for name, data := range site.Files {
		if strings.HasSuffix(name, ".html") {
			checkHTML(t, name, string(data))
		}
	}
	for name, data := range site.Files {
		if strings.HasPrefix(name, "items/") {
			return string(data)
		}
	}
	t.Fatal("no item page")
	return ""
}

// mdPart is the rendered Markdown of an item page: from its container to the end
// of <main>, before the footer's script tag.
func mdPart(page string) string {
	i := strings.Index(page, `<div class="md">`)
	if i < 0 {
		return ""
	}
	rest := page[i:]
	if j := strings.Index(rest, "</main>"); j >= 0 {
		rest = rest[:j]
	}
	return rest
}

func TestMarkdown_RendersTheSafeSubset(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []string
	}{
		{"paragraph and emphasis", "Hello *there* **you** `x < y`", []string{`<p dir="auto">Hello <em>there</em> <strong>you</strong> <code>x &lt; y</code></p>`}},
		{"headings sit below the page headings", "# One\n\n## Two\n\n###### Six\n\n####### not a heading", []string{"<h3 dir=\"auto\">One</h3>", "<h4 dir=\"auto\">Two</h4>", "<h6 dir=\"auto\">Six</h6>"}},
		{"lists", "- a\n- b\n\n3. c\n4. d", []string{"<ul>", "<li dir=\"auto\">a</li>", `<ol start="3">`, "<li dir=\"auto\">d</li>"}},
		{"code block", "```go\nfmt.Println(\"<b>\")\n```", []string{`<pre dir="auto">fmt.Println(&#34;&lt;b&gt;&#34;)</pre>`}},
		{"quote and rule", "> quoted\n\n---\n", []string{"<blockquote dir=\"auto\">", "<hr>"}},
		{"escapes and references are decoded once", `5 &amp; 6 &lt; 7 \*lit\* &#65;`, []string{"5 &amp; 6 &lt; 7 *lit* A"}},
		{"hard break", "line one  \nline two", []string{"line one<br>line two"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			page := renderExcerpt(t, tt.body)

			// Assert
			for _, want := range tt.want {
				assert.Contains(t, page, want)
			}
			assert.Contains(t, page, `<div class="md">`)
			assert.NotContains(t, page, "<pre dir=\"auto\">"+tt.body, "the raw text is not shown instead")
		})
	}
}

func TestMarkdown_NeverEmitsActiveContent(t *testing.T) {
	tests := []struct {
		name string
		body string
		// shown is text that must appear (as text), proving the input was kept for the reviewer.
		shown string
	}{
		{"script element", "<script>alert(1)</script>", "&lt;script&gt;alert(1)&lt;/script&gt;"},
		{"inline script", "text <script>alert(1)</script> text", "&lt;script&gt;"},
		{"image with handler", `<img src=x onerror=alert(1)>`, "&lt;img src=x onerror=alert(1)&gt;"},
		{"markdown image", "![alt text](https://evil.example/x.png)", "[image: alt text]"},
		{"javascript link", "[click](javascript:alert(1))", "javascript:alert(1)"},
		{"data link", "[click](data:text/html,<script>alert(1)</script>)", "data:text/html"},
		{"remote link", "[docs](https://example.com/a)", "https://example.com/a"},
		{"autolink", "<https://example.com/auto>", "https://example.com/auto"},
		{"reference link", "[ref][1]\n\n[1]: javascript:alert(2)", "javascript:alert(2)"},
		{"html block with handler", "<div onclick=\"alert(1)\">x</div>", "&lt;div onclick="},
		{"iframe", "<iframe src=\"//evil\"></iframe>", "&lt;iframe"},
		{"style attribute smuggling", `<p style="background:url(//evil)">x</p>`, "&lt;p style="},
		{"entity-encoded script", "&lt;script&gt;alert(1)&lt;/script&gt;", "&lt;script&gt;"},
		{"comment", "<!-- hidden -->visible", "visible"},
		{"bidi and zero width", "evil\u202e text\u200b here", "evil"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			page := renderExcerpt(t, tt.body)

			// Assert
			assert.Contains(t, page, tt.shown)
			assert.NotRegexp(t, `(?i)<(script|iframe|img|object|embed|style|svg|math|form|input)\b[^>]*>`, mdPart(page), "no active element in the rendered excerpt")
			assert.NotContains(t, mdPart(page), " href=", "a Markdown link is never an anchor")
			assert.NotContains(t, page, "\u202e")
			assert.NotContains(t, page, "\u200b")
		})
	}
}

func TestMarkdown_IsBounded(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"deep quotes", strings.Repeat("> ", 500) + "deep"},
		{"deep lists", strings.Repeat("  ", 200) + "- x"},
		{"many nodes", strings.Repeat("*a* ", maxMarkdownNodes)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			doc := manyItemsDoc(1)
			doc.Items[0].Excerpt = &govview.Excerpt{Text: tt.body}
			site, err := Render(doc, Options{Markdown: true})

			// Assert
			require.NoError(t, err)
			for name, data := range site.Files {
				if strings.HasSuffix(name, ".html") {
					checkHTML(t, name, string(data))
				}
			}
		})
	}
	t.Run("node cap says so", func(t *testing.T) {
		// Act
		nodes := renderMarkdown(strings.Repeat("*a* ", maxMarkdownNodes))

		// Assert
		last := nodes[len(nodes)-1]
		assert.Equal(t, "[rendering stopped: the text is too long]", last.Children[0].Text)
	})
}

func TestMarkdown_OffByDefaultAndPlainTextStaysPlain(t *testing.T) {
	// Arrange
	doc := manyItemsDoc(1)
	doc.Items[0].Excerpt = &govview.Excerpt{Text: "# Title\n<b>x</b>"}

	// Act
	site, err := Render(doc, Options{})

	// Assert
	require.NoError(t, err)
	var page string
	for name, data := range site.Files {
		if strings.HasPrefix(name, "items/") {
			page = string(data)
		}
	}
	assert.NotContains(t, page, `<div class="md">`)
	assert.Contains(t, page, "# Title\n&lt;b&gt;x&lt;/b&gt;")
}

func TestMarkdown_IsDeterministic(t *testing.T) {
	// Arrange
	body := "# T\n\n- a\n- b\n\n> q\n\n[l](https://x.y)\n"

	// Act
	first, second := renderExcerpt(t, body), renderExcerpt(t, body)

	// Assert
	assert.Equal(t, first, second)
}

func FuzzMarkdown_NeverEmitsActiveContent(f *testing.F) {
	for _, seed := range []string{
		"# h", "<script>x</script>", "[a](javascript:alert(1))", "![i](x)", "```\n<b>\n```", "> > > q", "- a\n  - b\n    - c",
		"<https://x.y>", "\x00\xff", "&#60;script&#62;", "*a **b** c*", "[x]: <y>\n[x]",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, body string) {
		doc := manyItemsDoc(1)
		doc.Items[0].Excerpt = &govview.Excerpt{Text: body}
		site, err := Render(doc, Options{Markdown: true})
		if err != nil {
			t.Fatal(err)
		}
		for name, data := range site.Files {
			if strings.HasPrefix(name, "items/") {
				checkHTML(t, name, string(data))
				assert.NotContains(t, mdPart(string(data)), " href=")
			}
		}
	})
}
