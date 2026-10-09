// Package frontmatter splits a leading YAML frontmatter block from a markdown
// document. It is the single definition of where that block ends, so every
// reader (lint, the served catalog, the importer, evals) agrees on one file.
package frontmatter

import "strings"

const (
	fence = "---"
	bom   = "\xef\xbb\xbf"
)

// Block is the result of Split.
type Block struct {
	// Raw is the YAML between the fences, LF-terminated per line, without the
	// fences. Empty for an empty block or when Present is false.
	Raw string
	// Head is the original text from the opening fence through the closing fence
	// (no trailing line terminator). Empty unless Closed.
	Head string
	// Tail is the original text after Head, including the line terminator that
	// ends the closing fence line. Empty unless Closed.
	Tail string
	// Body is the text after the closing fence line, with its original line
	// endings. When the document has no closed block it is the whole text
	// (minus a UTF-8 BOM).
	Body string
	// BodyLine is the 1-based line number where Body starts. Zero unless Closed.
	BodyLine int
	// Present is true when the document opens with a --- line, closed or not.
	Present bool
	// Closed is true when a closing --- line was found.
	Closed bool
}

// Split separates a leading frontmatter block from src.
//
// The rules, shared by every caller:
//   - a UTF-8 BOM before the opening fence is ignored and dropped;
//   - CRLF line ends are tolerated;
//   - a fence is a line that is exactly --- (trailing spaces, tabs and a CR are
//     ignored), so a longer dash run, "---foo" or a table rule never closes the
//     block;
//   - the closing fence is the first such line after the opening one, so the
//     empty block "---\n---\n" is empty rather than running on to a later rule;
//   - an unclosed block is Present but not Closed and leaves Body as the whole text.
func Split(src []byte) Block { return SplitString(string(src)) }

// SplitString is Split for a string.
func SplitString(src string) Block {
	text := strings.TrimPrefix(src, bom)
	span, ok := locate(text)
	if !ok {
		return Block{Body: text}
	}
	b := Block{Present: true, Body: text}
	var raw strings.Builder
	for lines := text[span.openEnd:span.rawEnd]; lines != ""; {
		var cur string
		cur, lines, _ = strings.Cut(lines, "\n")
		raw.WriteString(strings.TrimSuffix(cur, "\r"))
		raw.WriteByte('\n')
	}
	b.Raw = raw.String()
	if span.closed {
		b.Head = text[:span.headEnd]
		b.Tail = text[span.headEnd:]
		b.Body = text[min(span.headEnd+1, len(text)):]
		b.BodyLine = span.closeLine + 1
		b.Closed = true
	}
	return b
}

// ClosingLine returns the 1-based line number of the fence that closes the
// leading frontmatter block of src, by the rules of Split, without building the
// Block. It reports false when src has no block or the block is not closed. The
// body starts on the line after it, so the closing line number is also the
// 0-based index of the first body line.
func ClosingLine(src string) (line int, ok bool) {
	span, ok := locate(strings.TrimPrefix(src, bom))
	if !ok || !span.closed {
		return 0, false
	}
	return span.closeLine, true
}

// span is where the leading block lies in a text that starts after any BOM.
type span struct {
	openEnd   int // offset just past the opening fence line, terminator included
	rawEnd    int // offset where the YAML lines end: the closing fence line, or the last line terminator
	headEnd   int // offset just past the closing fence text (Closed only)
	closeLine int // 1-based line of the closing fence (Closed only)
	closed    bool
}

// locate finds the opening fence and the first closing fence after it. It
// reports false when text does not open with a fence line. It never copies text.
func locate(text string) (span, bool) {
	first, rest, ok := strings.Cut(text, "\n")
	if !ok || !isFence(first) {
		return span{}, false
	}
	sp := span{openEnd: len(first) + 1}
	offset, line := sp.openEnd, 2 // byte offset in text, and number, of the line being examined
	for {
		cur, tail, more := strings.Cut(rest, "\n")
		if isFence(cur) {
			sp.rawEnd, sp.headEnd, sp.closeLine, sp.closed = offset, offset+len(cur), line, true
			return sp, true
		}
		if !more {
			// Unclosed: the YAML is every complete line, without the last unterminated one.
			sp.rawEnd = offset
			return sp, true
		}
		offset += len(cur) + 1
		rest = tail
		line++
	}
}

func isFence(line string) bool {
	return strings.TrimRight(line, " \t\r") == fence
}

// TrimSeparator drops the single blank line conventionally written between the
// closing fence and the body.
func TrimSeparator(body string) string {
	return strings.TrimPrefix(strings.TrimPrefix(body, "\r\n"), "\n")
}
