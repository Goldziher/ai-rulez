package lint

import (
	"regexp"
	"strings"
	"sync"
)

// Codes for the markdown shape checks. Both have a safe fix (`validate --fix`).
const (
	CodeFenceUnclosed = "AR806"
	CodeFinalNewline  = "AR807"
)

func registerArMarkdown(s *ruleSet) {
	s.addRules(
		RuleInfo{CodeFenceUnclosed, "fence-unclosed", SeverityWarning, "a fenced code block is opened and never closed, so every line after it is read as code"},
		RuleInfo{CodeFinalNewline, "final-newline-missing", SeverityInfo, "a content file does not end with a newline"},
	)
	s.addDocs(map[string]RuleDoc{
		CodeFenceUnclosed: {
			Why:  "Past an unclosed fence every line is code: links, names and headings stop being read as prose, and a harness may render or load the rest of the file differently. It is almost always a hand-edit slip. `validate --fix` closes the fence at the end of the file; check that is where the block should end.",
			Bad:  "A file whose last block starts with ```` ```bash ```` and has no closing ```` ``` ````",
			Good: "Close the block with a fence of the same character and at least the same length",
		},
		CodeFinalNewline: {
			Why:  "Tools disagree about a last line without a newline: diffs show `\\ No newline at end of file`, concatenated or appended text lands on the same line, and formatters rewrite the file. `validate --fix` adds the newline (CRLF files get CRLF).",
			Bad:  "A rule file whose last byte is not a line feed",
			Good: "End the file with exactly one newline",
		},
	})
}

// unclosedFence returns the opening run and 1-based line of a fenced block that
// is still open at the end of the document, using the same rules as doc.body.
func unclosedFence(d doc) (run string, line int, open bool) {
	var cur string
	for i := d.bodyStart; i < len(d.lines); i++ {
		m := fenceRe().FindStringSubmatch(d.lines[i])
		if m == nil {
			continue
		}
		fence, info := m[1], strings.TrimSpace(m[2])
		switch {
		case cur == "":
			if fence[0] != '`' || !strings.Contains(info, "`") {
				cur, line = fence, i+1
			}
		case fence[0] == cur[0] && len(fence) >= len(cur) && info == "":
			cur = ""
		}
	}
	return cur, line, cur != ""
}

// checkMarkdownShape reports an unclosed fence (AR806) and a missing final
// newline (AR807). Both are repaired by one edit of the last line, so a file
// with both problems is fixed once and the two fixes do not conflict.
func (r *runner) checkMarkdownShape(it *item, d doc, raw string) {
	if strings.TrimSpace(raw) == "" || len(d.lines) == 0 {
		return
	}
	endsNewline := strings.HasSuffix(raw, "\n")
	last := len(d.lines) - 1
	if endsNewline {
		last--
	}
	if last < 0 {
		return
	}
	edit := Edit{File: it.abs, Line: last + 1, Old: d.lines[last], New: d.lines[last], FinalNewline: true}
	run, openLine, open := unclosedFence(d)
	if open {
		edit.After = strings.Repeat(string(run[0]), len(run))
		fix := &Fix{Description: "close the fence at the end of the file", Confidence: FixSafe, Edits: []Edit{edit}}
		r.addFix(fix, CodeFenceUnclosed, it.abs, openLine, "the code fence opened here (%s) is never closed", run)
	}
	if !endsNewline {
		fix := &Fix{Description: "add the missing final newline", Confidence: FixSafe, Edits: []Edit{edit}}
		r.addFix(fix, CodeFinalNewline, it.abs, last+1, "the file does not end with a newline")
	}
}

var quotedBoolRe = sync.OnceValue(func() *regexp.Regexp {
	return regexp.MustCompile(`(?i)^(\s*[^:]+:\s*)(["'])(true|false)(["'])(\s*(?:#.*)?)$`)
})

// boolCoercion is the fix for a boolean frontmatter key written as a quoted
// string: the same line with the value unquoted. nil when the line is not that
// shape (a block scalar, a flow mapping, mismatched quotes).
func boolCoercion(it *item, d doc, k fmKey) *Fix {
	if k.Line < 1 || k.Line > len(d.lines) {
		return nil
	}
	line := d.lines[k.Line-1]
	m := quotedBoolRe().FindStringSubmatch(line)
	if len(m) == 0 || m[2] != m[4] || !strings.Contains(m[1], k.Name) {
		return nil
	}
	return &Fix{
		Description: "write " + strings.ToLower(m[3]) + " as a YAML boolean, without quotes",
		Confidence:  FixSafe,
		Edits:       []Edit{{File: it.abs, Line: k.Line, Old: line, New: m[1] + strings.ToLower(m[3]) + m[5]}},
	}
}
