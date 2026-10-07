package llmstxt

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Finding is one problem in an llms.txt file.
type Finding struct {
	Code     string   `json:"code"`
	Name     string   `json:"name"`
	Severity Severity `json:"severity"`
	Line     int      `json:"line"`
	Message  string   `json:"message"`
}

// OptionalSection is the conventional name of the section of skippable links.
const OptionalSection = "Optional"

var (
	headingRe  = regexp.MustCompile(`^(#{1,6})(?:[ \t]+(.*?))?[ \t]*$`)
	listItemRe = regexp.MustCompile(`^(?:[-*+]|\d{1,9}[.)])[ \t]+(.*)$`)
	linkRe     = regexp.MustCompile(`\[(?:\\.|[^\]\\])*\]\(([^)]*)\)`)
	titleRe    = regexp.MustCompile(`\s+"[^"]*"$`)
)

type section struct {
	name  string
	line  int
	items int
}

type validator struct {
	out         []Finding
	sections    []section
	inSection   bool
	seenTitle   bool
	titleBad    bool
	afterTitle  bool // only the H1 has been seen
	inSummary   bool // the previous content line was part of the summary
	summaryText strings.Builder
	summaryLine int
}

func (v *validator) add(code string, line int, format string, args ...any) {
	v.out = append(v.out, Finding{Code: code, Name: ruleName(code), Severity: ruleSeverity(code), Line: line, Message: fmt.Sprintf(format, args...)})
}

// Validate checks src against the llms.txt format. The findings are ordered by
// line and code so they can be diffed.
func Validate(src []byte) []Finding {
	text := strings.TrimPrefix(string(src), "\ufeff")
	v := &validator{}
	fence := ""
	first := true
	for i, raw := range strings.Split(text, "\n") {
		line := strings.TrimRight(raw, "\r \t")
		no := i + 1
		if strings.TrimSpace(line) == "" {
			v.endSummary()
			continue
		}
		if marker := fenceMarker(line); marker != "" {
			if fence == "" {
				fence = marker
			} else if strings.HasPrefix(strings.TrimSpace(line), fence) {
				fence = ""
			}
			v.content(no, first, false)
			first = false
			continue
		}
		if fence != "" {
			continue
		}
		v.line(no, line, first)
		first = false
	}
	v.endSummary()
	if first {
		v.add(CodeTitleMissing, 1, "the file is empty; it must start with an H1 title")
	}
	v.checkSections()
	sort.SliceStable(v.out, func(a, b int) bool {
		if v.out[a].Line != v.out[b].Line {
			return v.out[a].Line < v.out[b].Line
		}
		return v.out[a].Code < v.out[b].Code
	})
	return v.out
}

func fenceMarker(line string) string {
	t := strings.TrimSpace(line)
	switch {
	case strings.HasPrefix(t, "```"):
		return "```"
	case strings.HasPrefix(t, "~~~"):
		return "~~~"
	}
	return ""
}

func (v *validator) line(no int, line string, first bool) {
	if m := headingRe.FindStringSubmatch(line); m != nil {
		v.heading(no, len(m[1]), strings.TrimSpace(m[2]), first)
		return
	}
	if first {
		v.add(CodeTitleMissing, no, "the file must start with an H1 title, found %q", clip(line))
		v.titleBad = true
	}
	if strings.HasPrefix(line, ">") && !v.inSection {
		v.summary(no, line)
		return
	}
	v.endSummary()
	if v.inSection {
		v.sectionLine(no, line)
		return
	}
	v.afterTitle = false
}

// content records a fence line, which is content of whatever block it is in.
func (v *validator) content(no int, first, _ bool) {
	if first {
		v.add(CodeTitleMissing, no, "the file must start with an H1 title")
		v.titleBad = true
	}
	v.endSummary()
	v.afterTitle = false
	if v.inSection {
		v.sections[len(v.sections)-1].items++
	}
}

func (v *validator) heading(no, level int, name string, first bool) {
	v.endSummary()
	switch {
	case level == 1:
		switch {
		case v.seenTitle:
			v.add(CodeTitleMissing, no, "a second H1 %q; only one title is allowed", clip(name))
		case !first:
			// The title came late; the first line already reported it.
			v.seenTitle = true
		default:
			v.seenTitle = true
			v.afterTitle = true
		}
		v.inSection = false
	case level == 2:
		if first {
			v.add(CodeTitleMissing, no, "the file must start with an H1 title, found an H2")
			v.titleBad = true
		}
		v.afterTitle = false
		v.inSection = true
		if name == "" {
			v.add(CodeHeadingInvalid, no, "an H2 section needs a name")
		}
		v.sections = append(v.sections, section{name: name, line: no})
	default:
		if first {
			v.add(CodeTitleMissing, no, "the file must start with an H1 title, found an H%d", level)
			v.titleBad = true
		}
		v.afterTitle = false
		v.add(CodeHeadingInvalid, no, "an H%d heading %q is not allowed; only the H1 title and H2 section names are", level, clip(name))
	}
}

func (v *validator) summary(no int, line string) {
	if !v.afterTitle && !v.inSummary {
		v.add(CodeSummaryMisplaced, no, "the summary blockquote must directly follow the H1 title")
		return
	}
	if !v.inSummary {
		v.summaryLine = no
	}
	v.inSummary = true
	v.summaryText.WriteString(strings.TrimSpace(strings.TrimPrefix(line, ">")))
}

func (v *validator) endSummary() {
	if !v.inSummary {
		return
	}
	if v.summaryText.Len() == 0 {
		v.add(CodeSummaryMisplaced, v.summaryLine, "the summary blockquote is empty")
	}
	v.inSummary = false
	v.afterTitle = false
	v.summaryText.Reset()
}

func (v *validator) sectionLine(no int, line string) {
	if line[0] == ' ' || line[0] == '\t' {
		// Continuation of the previous entry (or a nested list).
		return
	}
	cur := &v.sections[len(v.sections)-1]
	m := listItemRe.FindStringSubmatch(line)
	if m == nil {
		v.add(CodeLinkEntryInvalid, no, "a file-list section holds markdown list entries, found %q", clip(line))
		return
	}
	cur.items++
	link := linkRe.FindStringSubmatch(m[1])
	if link == nil {
		v.add(CodeLinkEntryInvalid, no, "the list entry %q has no [name](url) link", clip(m[1]))
		return
	}
	target := strings.TrimSpace(titleRe.ReplaceAllString(strings.TrimSpace(link[1]), ""))
	target = strings.TrimSuffix(strings.TrimPrefix(target, "<"), ">")
	switch {
	case target == "":
		v.add(CodeLinkTargetInvalid, no, "the link %q has an empty target", clip(m[1]))
	case strings.ContainsAny(target, " \t"):
		v.add(CodeLinkEntryInvalid, no, "the link target %q contains whitespace; encode it as %%20", clip(target))
	}
}

func (v *validator) checkSections() {
	seen := map[string]int{}
	for i, s := range v.sections {
		if s.name == "" {
			continue
		}
		if _, dup := seen[s.name]; dup {
			v.add(CodeSectionDuplicateOrEmpty, s.line, "the section %q repeats the one at line %d", s.name, seen[s.name])
		} else {
			seen[s.name] = s.line
		}
		if s.items == 0 {
			v.add(CodeSectionDuplicateOrEmpty, s.line, "the section %q has no entries", s.name)
		}
		if s.name == OptionalSection && i != len(v.sections)-1 {
			v.add(CodeOptionalMisplaced, s.line, "the %q section must come last so agents can drop it as a block", OptionalSection)
		}
	}
}

func clip(s string) string {
	const limit = 60
	if r := []rune(s); len(r) > limit {
		return string(r[:limit]) + "..."
	}
	return s
}
