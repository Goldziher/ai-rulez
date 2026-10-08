package llmstxt

import (
	"regexp"
	"strings"
)

// Link is one entry of a file-list section.
type Link struct {
	Title string
	URL   string
	// Note is the one-line description after the link.
	Note string
}

// Section is an H2 file-list section.
type Section struct {
	Name  string
	Links []Link
}

// Doc is the content of an llms.txt file.
type Doc struct {
	Title   string
	Summary string
	// Details is free markdown without headings, shown between the summary and
	// the sections.
	Details  string
	Sections []Section
}

// Render writes the document. Sections keep their order except "Optional",
// which moves last, and sections without links are dropped. The output ends in
// one newline.
func (d Doc) Render() string {
	var sb strings.Builder
	title := oneLine(d.Title)
	if title == "" {
		title = "Untitled"
	}
	sb.WriteString("# " + title + "\n")
	if summary := oneLine(d.Summary); summary != "" {
		sb.WriteString("\n> " + summary + "\n")
	}
	if details := strings.TrimSpace(strings.ReplaceAll(d.Details, "\r\n", "\n")); details != "" {
		sb.WriteString("\n" + details + "\n")
	}
	var optional []Section
	for _, s := range d.Sections {
		if len(s.Links) == 0 {
			continue
		}
		if s.Name == OptionalSection {
			optional = append(optional, s)
			continue
		}
		writeSection(&sb, s)
	}
	for _, s := range optional {
		writeSection(&sb, s)
	}
	return sb.String()
}

func writeSection(sb *strings.Builder, s Section) {
	sb.WriteString("\n## " + oneLine(s.Name) + "\n\n")
	for _, l := range s.Links {
		sb.WriteString("- [" + escapeTitle(oneLine(l.Title)) + "](" + escapeTarget(l.URL) + ")")
		if note := escapeText(oneLine(l.Note)); note != "" {
			sb.WriteString(": " + note)
		}
		sb.WriteString("\n")
	}
}

// oneLine collapses all whitespace runs to single spaces.
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// textEscaper backslash-escapes the characters that start a link, an image or
// raw HTML, so untrusted text renders as text.
var textEscaper = strings.NewReplacer(`\`, `\\`, "[", `\[`, "]", `\]`, "<", `\<`, ">", `\>`, "!", `\!`)

func escapeTitle(s string) string { return textEscaper.Replace(oneLine(s)) }

func escapeText(s string) string { return textEscaper.Replace(s) }

var targetEscaper = strings.NewReplacer(
	" ", "%20", "(", "%28", ")", "%29", "\t", "%09", "\n", "%0A", "\r", "%0D", "<", "%3C", ">", "%3E",
	"\v", "%0B", "\f", "%0C",
)

func escapeTarget(s string) string {
	return targetEscaper.Replace(strings.TrimSpace(s))
}

// Page is one document of an llms-full.txt file.
type Page struct {
	Title string
	// Source names where the page came from (a path or URL); optional.
	Source string
	Body   string
}

// RenderFull writes the expanded form: the title and summary, then every page
// as an H2 section holding its full text. Headings inside a page are demoted
// by two levels so they nest under the page heading (levels past 6 stay at 6);
// fenced code is left alone.
func RenderFull(title, summary string, pages []Page) string {
	var sb strings.Builder
	t := oneLine(title)
	if t == "" {
		t = "Untitled"
	}
	sb.WriteString("# " + t + "\n")
	if s := oneLine(summary); s != "" {
		sb.WriteString("\n> " + s + "\n")
	}
	for _, p := range pages {
		sb.WriteString("\n## " + oneLine(p.Title) + "\n\n")
		if src := oneLine(p.Source); src != "" {
			sb.WriteString("Source: " + src + "\n\n")
		}
		body := strings.TrimSpace(demote(strings.ReplaceAll(p.Body, "\r\n", "\n")))
		if body != "" {
			sb.WriteString(body + "\n")
		}
	}
	return sb.String()
}

// setextRe matches a setext underline: a run of = or - (up to three spaces of indent).
var setextRe = regexp.MustCompile(`^ {0,3}(=+|-+)[ \t]*$`)

// atxFromSetext rewrites setext headings (a text line over a === or --- line) as
// ATX headings so demote can see them. Fenced code is left alone; a paragraph
// line that looks like a list item, quote or indented code is not a heading text.
func atxFromSetext(lines []string) {
	fence := ""
	for i, line := range lines {
		if marker := fenceMarker(line); marker != "" {
			switch {
			case fence == "":
				fence = marker
			case marker == fence:
				fence = ""
			}
			continue
		}
		if fence != "" || i == 0 {
			continue
		}
		m := setextRe.FindStringSubmatch(line)
		prev := lines[i-1]
		if m == nil || strings.TrimSpace(prev) == "" || fenceMarker(prev) != "" || headingRe.MatchString(prev) ||
			strings.HasPrefix(prev, "    ") || strings.HasPrefix(prev, "\t") || setextRe.MatchString(prev) ||
			listItemRe.MatchString(strings.TrimSpace(prev)) || strings.HasPrefix(strings.TrimSpace(prev), ">") {
			continue
		}
		level := "#"
		if m[1][0] == '-' {
			level = "##"
		}
		lines[i-1] = level + " " + strings.TrimSpace(prev)
		lines[i] = ""
	}
}

func demote(body string) string {
	lines := strings.Split(body, "\n")
	atxFromSetext(lines)
	fence := ""
	for i, line := range lines {
		if marker := fenceMarker(line); marker != "" {
			switch {
			case fence == "":
				fence = marker
			case marker == fence:
				fence = ""
			}
			continue
		}
		if fence != "" {
			continue
		}
		if m := headingRe.FindStringSubmatch(line); m != nil {
			level := min(len(m[1])+2, 6)
			lines[i] = strings.Repeat("#", level) + line[len(m[1]):]
		}
	}
	return strings.Join(lines, "\n")
}
