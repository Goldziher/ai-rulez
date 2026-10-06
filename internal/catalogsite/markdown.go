package catalogsite

import (
	"bufio"
	"bytes"
	stdhtml "html"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
)

// Bounds on a rendered Markdown tree, so a hostile body cannot make a page
// enormous or recurse without end.
const (
	maxMarkdownDepth = 12
	maxMarkdownNodes = 4000
	// headingShift moves Markdown headings below the page's own h1 and h2: "#"
	// becomes an h3, and anything deeper than an h6 stays an h6.
	headingShift = 2
	maxHeading   = 6
)

// Kinds of mdNode. Rendering switches on these and on nothing else, so a Markdown
// construct without a kind here can never become an element.
const (
	mdParagraph  = "p"
	mdHeading    = "h"
	mdQuote      = "quote"
	mdList       = "list"
	mdItem       = "item"
	mdCodeBlock  = "pre"
	mdRule       = "hr"
	mdText       = "text"
	mdCode       = "code"
	mdEmphasis   = "em"
	mdStrong     = "strong"
	mdBreak      = "br"
	mdLinkTarget = "target"
)

// mdNode is one node of a sanitized Markdown tree. Text is plain text that the
// template escapes; there is no field that holds markup, so nothing in the tree
// can inject any.
type mdNode struct {
	Kind     string
	Text     string
	Level    int
	Ordered  bool
	Start    int
	Children []*mdNode
}

// renderMarkdown parses src as CommonMark and returns a tree of allowlisted
// nodes. It is a sanitizer by construction rather than by filtering: raw HTML
// (blocks and inline) becomes literal text, images become their alt text, links
// keep their label and show the destination as text instead of linking to it
// (so no javascript:, data: or remote URL is ever an attribute), and every
// other construct is dropped to its text content.
func renderMarkdown(src string) []*mdNode {
	source := []byte(strings.ToValidUTF8(strings.ReplaceAll(src, "\r\n", "\n"), "�"))
	doc := goldmark.New().Parser().Parse(text.NewReader(source))
	b := &mdBuilder{src: source}
	root := &mdNode{}
	b.children(doc, root, 0)
	if b.count > maxMarkdownNodes {
		root.Children = append(root.Children, &mdNode{Kind: mdParagraph, Children: []*mdNode{{Kind: mdText, Text: "[rendering stopped: the text is too long]"}}})
	}
	return root.Children
}

type mdBuilder struct {
	src   []byte
	count int
}

func (b *mdBuilder) add(parent *mdNode, n *mdNode) *mdNode {
	b.count++
	parent.Children = append(parent.Children, n)
	return n
}

func (b *mdBuilder) children(n ast.Node, into *mdNode, depth int) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if b.count > maxMarkdownNodes {
			return
		}
		b.node(c, into, depth)
	}
}

func (b *mdBuilder) node(n ast.Node, into *mdNode, depth int) {
	if depth >= maxMarkdownDepth {
		b.add(into, &mdNode{Kind: mdText, Text: b.plain(n, true)})
		return
	}
	switch v := n.(type) {
	case *ast.Paragraph:
		b.children(n, b.add(into, &mdNode{Kind: mdParagraph}), depth+1)
	case *ast.TextBlock:
		// The text of a tight list item: no paragraph of its own.
		b.children(n, into, depth+1)
	case *ast.Heading:
		b.children(n, b.add(into, &mdNode{Kind: mdHeading, Level: min(v.Level+headingShift, maxHeading)}), depth+1)
	case *ast.Blockquote:
		b.children(n, b.add(into, &mdNode{Kind: mdQuote}), depth+1)
	case *ast.List:
		list := &mdNode{Kind: mdList, Ordered: v.IsOrdered(), Start: v.Start}
		b.children(n, b.add(into, list), depth+1)
	case *ast.ListItem:
		b.children(n, b.add(into, &mdNode{Kind: mdItem}), depth+1)
	case *ast.FencedCodeBlock, *ast.CodeBlock:
		b.add(into, &mdNode{Kind: mdCodeBlock, Text: b.lines(n)})
	case *ast.HTMLBlock:
		// Raw HTML is shown, never interpreted.
		b.add(into, &mdNode{Kind: mdCodeBlock, Text: b.lines(n)})
	case *ast.ThematicBreak:
		b.add(into, &mdNode{Kind: mdRule})
	case *ast.Emphasis:
		kind := mdEmphasis
		if v.Level >= 2 {
			kind = mdStrong
		}
		b.children(n, b.add(into, &mdNode{Kind: kind}), depth+1)
	case *ast.CodeSpan:
		b.add(into, &mdNode{Kind: mdCode, Text: b.plain(n, false)})
	case *ast.Text:
		b.add(into, &mdNode{Kind: mdText, Text: decodeText(v.Segment.Value(b.src))})
		switch {
		case v.HardLineBreak():
			b.add(into, &mdNode{Kind: mdBreak})
		case v.SoftLineBreak():
			b.add(into, &mdNode{Kind: mdText, Text: " "})
		}
	case *ast.String:
		b.add(into, &mdNode{Kind: mdText, Text: string(v.Value)})
	case *ast.RawHTML:
		b.add(into, &mdNode{Kind: mdCode, Text: b.rawSegments(v)})
	case *ast.Link:
		b.children(n, into, depth+1)
		b.add(into, &mdNode{Kind: mdText, Text: " ("})
		b.add(into, &mdNode{Kind: mdLinkTarget, Text: string(v.Destination)})
		b.add(into, &mdNode{Kind: mdText, Text: ")"})
	case *ast.AutoLink:
		b.add(into, &mdNode{Kind: mdLinkTarget, Text: string(v.URL(b.src))})
	case *ast.Image:
		b.add(into, &mdNode{Kind: mdText, Text: "[image: " + b.plain(n, true) + "] ("})
		b.add(into, &mdNode{Kind: mdLinkTarget, Text: string(v.Destination)})
		b.add(into, &mdNode{Kind: mdText, Text: ")"})
	default:
		// A construct without a kind (for example a link reference definition)
		// contributes its text only.
		b.children(n, into, depth+1)
	}
}

// lines joins the source lines of a block node.
func (b *mdBuilder) lines(n ast.Node) string {
	var sb strings.Builder
	segs := n.Lines()
	for i := 0; i < segs.Len(); i++ {
		seg := segs.At(i)
		sb.Write(seg.Value(b.src))
	}
	return strings.TrimRight(sb.String(), "\n")
}

func (b *mdBuilder) rawSegments(n *ast.RawHTML) string {
	var sb strings.Builder
	for i := 0; i < n.Segments.Len(); i++ {
		seg := n.Segments.At(i)
		sb.Write(seg.Value(b.src))
	}
	return sb.String()
}

// decodeText resolves the backslash escapes and character references of a text
// segment the way CommonMark does, giving the characters the author meant. The
// result is plain text: the template escapes it again.
func decodeText(raw []byte) string {
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	html.DefaultWriter.Write(w, raw)
	_ = w.Flush() //nolint:errcheck // a bytes.Buffer never fails
	return stdhtml.UnescapeString(buf.String())
}

// plain is the text content of a node and its descendants; decode resolves
// escapes and references (code spans keep their text as written).
func (b *mdBuilder) plain(n ast.Node, decode bool) string {
	var sb strings.Builder
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) { //nolint:errcheck // the walker never fails
		if !entering {
			return ast.WalkContinue, nil
		}
		switch t := c.(type) {
		case *ast.Text:
			if decode {
				sb.WriteString(decodeText(t.Segment.Value(b.src)))
			} else {
				sb.Write(t.Segment.Value(b.src))
			}
		case *ast.String:
			sb.Write(t.Value)
		case *ast.RawHTML:
			sb.WriteString(b.rawSegments(t))
		}
		return ast.WalkContinue, nil
	})
	return sb.String()
}
