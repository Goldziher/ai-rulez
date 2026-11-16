package importer

import (
	"errors"
	"io/fs"
	"slices"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/frontmatter"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	mdtext "github.com/yuin/goldmark/text"
)

// agentsMDFile is the root instruction file the "agents" source reads.
const agentsMDFile = "AGENTS.md"

// AgentsMD is the root AGENTS.md read as the source of the "agents" source.
type AgentsMD struct {
	// Items are the context items the file yields: one, or one per H2 section
	// with Options.SplitHeadings.
	Items []Item
	// Notes say why the file was not used: a generated AGENTS.md is output of an
	// earlier run and never source, a symlink is not followed, an empty file has
	// nothing to read.
	Notes []Finding
}

// ReadAgentsMD reads the root AGENTS.md of the tree fsys is rooted at. It goes
// through the same reader and screening as `convert`: the file must be a regular
// file within the size limits, and a file carrying the generated header is
// refused so ai-rulez output is never read back as source.
func ReadAgentsMD(fsys fs.FS, opt Options) AgentsMD {
	p := &Plan{}
	r := newReader(fsys)
	if _, ok := r.exists(agentsMDFile); ok {
		nativeImporter{}.importRootFile(p, r, agentsMDFile, opt)
	}
	r.flushProblems(p)
	var out AgentsMD
	for i := range p.Items {
		if p.Items[i].Kind == KindContext {
			out.Items = append(out.Items, p.Items[i])
		}
	}
	out.Notes = p.Findings
	return out
}

// H2Span is one section of a Markdown text: the lines [Start, End) of the text
// split at "\n". A section starts at a top-level H2; nonblank text before the
// first H2 is a section too (Header "").
type H2Span struct {
	Header     string
	Start, End int
}

// H2Spans locates the sections ReadAgentsMD yields with SplitHeadings, in the
// same order, as line ranges of the text as written. A leading BOM is ignored,
// CRLF lines are accepted, and a bare CR is an error because the line ranges
// cannot address it.
func H2Spans(text string) ([]H2Span, error) {
	text = strings.TrimPrefix(text, "\ufeff")
	if strings.Contains(strings.ReplaceAll(text, "\r\n", ""), "\r") {
		return nil, bareCRError()
	}
	var spans []H2Span
	for _, section := range locateH2Sections(text) {
		spans = append(spans, section.H2Span)
	}
	return spans, nil
}

type locatedH2Section struct {
	H2Span
	bodyStart int // line after the heading; equal to Start for a preamble
}

func locateH2Sections(text string) []locatedH2Section {
	lines := strings.Split(text, "\n")
	starts := make([]int, len(lines))
	for i := 1; i < len(lines); i++ {
		starts[i] = starts[i-1] + len(lines[i-1]) + 1
	}
	offset := 0
	if first, ok := frontmatter.ClosingLine(text); ok {
		if first < len(starts) {
			offset = starts[first]
		} else {
			offset = len(text)
		}
	}
	source := []byte(text[offset:])
	doc := goldmark.DefaultParser().Parse(mdtext.NewReader(source))
	var sections []locatedH2Section
	for child := doc.FirstChild(); child != nil; child = child.NextSibling() {
		heading, ok := child.(*ast.Heading)
		if !ok || heading.Level != 2 {
			continue
		}
		start := sourceLine(starts, heading.Pos()+offset)
		if len(sections) == 0 && strings.TrimSpace(text[:starts[start]]) != "" {
			sections = append(sections, locatedH2Section{H2Span: H2Span{End: start}})
		}
		if len(sections) > 0 {
			sections[len(sections)-1].End = start
		}
		header, bodyStart := headingDetails(heading, source, offset, starts, lines[start], start)
		sections = append(sections, locatedH2Section{H2Span{Header: header, Start: start}, bodyStart})
	}
	if len(sections) > 0 {
		sections[len(sections)-1].End = len(lines)
	} else if strings.TrimSpace(text) != "" {
		sections = append(sections, locatedH2Section{H2Span: H2Span{End: len(lines)}})
	}
	return sections
}

func sourceLine(starts []int, offset int) int {
	i, exact := slices.BinarySearch(starts, offset)
	if exact {
		return i
	}
	return i - 1
}

func headingDetails(heading *ast.Heading, source []byte, offset int, starts []int, line string, start int) (string, int) {
	parts := make([]string, 0, heading.Lines().Len())
	bodyStart := start + 1
	for i := 0; i < heading.Lines().Len(); i++ {
		segment := heading.Lines().At(i)
		parts = append(parts, strings.TrimSpace(string(segment.Value(source))))
		bodyStart = sourceLine(starts, segment.Start+offset) + 1
	}
	// A Setext H2's AST stores its title lines but leaves out the underline.
	// An ATX H2 begins with two hashes followed by whitespace or end of line.
	line = strings.TrimLeft(line, " ")
	after, atx := strings.CutPrefix(line, "##")
	if !atx || (after != "" && !strings.ContainsRune(" \t\r\n", rune(after[0]))) {
		bodyStart++
	}
	return strings.Join(parts, " "), bodyStart
}

func bareCRError() error {
	return errors.New("the file uses lone CR line endings; convert them to LF or CRLF first")
}

// IsGenerated reports whether data, read from file, is ai-rulez output: it
// carries the generated header or banner of a preset. It is the check `convert`
// applies, exported for the "agents" source.
func IsGenerated(file string, data []byte) bool {
	return isGenerated(file, data)
}
