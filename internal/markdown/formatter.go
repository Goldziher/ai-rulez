package markdown

import (
	"bytes"
	"strings"

	markdown "github.com/teekennedy/goldmark-markdown"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
)

// Formatter formats markdown content to be consistent and markdownlint-compliant
type Formatter struct {
	md goldmark.Markdown
}

// NewFormatter creates a new markdown formatter
func NewFormatter() *Formatter {
	// Configure goldmark with markdown renderer
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,         // GitHub Flavored Markdown
			extension.Typographer, // Smart quotes, dashes, etc.
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(), // Add IDs to headings
		),
		goldmark.WithRenderer(
			markdown.NewRenderer(
				markdown.WithHeadingStyle(markdown.HeadingStyleATX), // Use # style headings
			),
		),
	)

	return &Formatter{md: md}
}

// Format formats markdown content to be consistent and compliant
func (f *Formatter) Format(content string) (string, error) {
	// Parse and render the markdown
	var buf bytes.Buffer
	if err := f.md.Convert([]byte(content), &buf); err != nil {
		return "", err
	}

	// Get the formatted output
	formatted := buf.String()

	// Ensure proper trailing newline
	formatted = strings.TrimRight(formatted, "\n") + "\n"

	return formatted, nil
}

// FormatString is a convenience function that formats markdown content
func FormatString(content string) (string, error) {
	f := NewFormatter()
	return f.Format(content)
}
