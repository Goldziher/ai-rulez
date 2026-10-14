package importer

import (
	"io/fs"
	"strings"
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
// split at "\n". A section starts at its "## " heading; the text before the
// first heading is a section too (Header ""), unless it is blank.
type H2Span struct {
	Header     string
	Start, End int
}

// H2Spans locates the sections ReadAgentsMD yields with SplitHeadings, in the
// same order, as line ranges of the text as written. A heading inside a code
// fence does not start a section.
func H2Spans(text string) []H2Span {
	lines := strings.Split(text, "\n")
	var spans []H2Span
	cur := H2Span{}
	blank := true
	var fenceChar byte
	fenceLen := 0
	flush := func(end int) {
		cur.End = end
		if cur.Header != "" || !blank {
			spans = append(spans, cur)
		}
	}
	for i, line := range lines {
		if ch, n, rest, ok := fenceLine(line); ok {
			switch {
			case fenceLen == 0:
				fenceChar, fenceLen = ch, n
			case ch == fenceChar && n >= fenceLen && strings.TrimSpace(rest) == "":
				fenceLen = 0
			}
		}
		if fenceLen == 0 && strings.HasPrefix(line, "## ") {
			flush(i)
			cur = H2Span{Header: strings.TrimSpace(strings.TrimPrefix(line, "## ")), Start: i}
			blank = true
			continue
		}
		if strings.TrimSpace(line) != "" {
			blank = false
		}
	}
	flush(len(lines))
	return spans
}

// IsGenerated reports whether data, read from file, is ai-rulez output: it
// carries the generated header or banner of a preset. It is the check `convert`
// applies, exported for the "agents" source.
func IsGenerated(file string, data []byte) bool {
	return isGenerated(file, data)
}
