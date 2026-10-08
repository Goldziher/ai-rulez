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
	first, rest, ok := strings.Cut(text, "\n")
	if !ok || !isFence(first) {
		return Block{Body: text}
	}
	b := Block{Present: true, Body: text}
	var raw strings.Builder
	offset := len(first) + 1 // byte offset in text of the line being examined
	line := 2
	for {
		cur, tail, more := strings.Cut(rest, "\n")
		if isFence(cur) {
			end := offset + len(cur)
			b.Raw = raw.String()
			b.Head = text[:end]
			b.Tail = text[end:]
			b.Body = text[min(end+1, len(text)):]
			b.BodyLine = line + 1
			b.Closed = true
			return b
		}
		if !more {
			break
		}
		raw.WriteString(strings.TrimSuffix(cur, "\r"))
		raw.WriteByte('\n')
		offset += len(cur) + 1
		rest = tail
		line++
	}
	b.Raw = raw.String()
	return b
}

func isFence(line string) bool {
	return strings.TrimRight(line, " \t\r") == fence
}

// TrimSeparator drops the single blank line conventionally written between the
// closing fence and the body.
func TrimSeparator(body string) string {
	return strings.TrimPrefix(strings.TrimPrefix(body, "\r\n"), "\n")
}
