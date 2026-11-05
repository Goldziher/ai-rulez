package areas

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/pathglob"
)

// Accept is a recorded acceptance of a known occurrence of an area word at session start: the word, the file it
// is in and why it is fine there. Index is the 0-based position in the areas file and Line the line of its
// [[accept]] header (0 when unknown).
type Accept struct {
	// Term is the accepted word, lowercased.
	Term string
	// Area optionally limits the acceptance to the words of one area.
	Area string
	// Path is a repository-relative path or glob, read as the path of a [[lint.accept]] entry is: "**" crosses
	// directories, a pattern with no slash matches at any depth, and a directory covers everything below it.
	Path   string
	Reason string
	Index  int
	Line   int
	glob   pathglob.Matcher
}

// Accepts returns the recorded acceptances in file order.
func (m *Map) Accepts() []Accept {
	if m == nil {
		return nil
	}
	out := slices.Clone(m.accepts)
	for i := range out {
		out[i].glob = pathglob.Matcher{}
	}
	return out
}

// Accepting returns the first acceptance of the word term in the file filePath. area is the area of the
// finding, or several joined by "/" for an ambiguous word that more than one area uses; an acceptance that
// names an area covers it only when it is among them.
func (m *Map) Accepting(area, term, filePath string) (Accept, bool) {
	if m == nil {
		return Accept{}, false
	}
	term = pyStrLower(strings.TrimFunc(term, pyIsSpace))
	for _, a := range m.accepts {
		if a.Term == term && (a.Area == "" || slices.Contains(strings.Split(area, "/"), a.Area)) && a.glob.Match(filePath) {
			return a, true
		}
	}
	return Accept{}, false
}

// normalizeAccepts validates the acceptances and lowercases their words; known are the area names.
func normalizeAccepts(accepts []Accept, known map[string]bool) ([]Accept, []Problem) {
	var problems []Problem
	out := make([]Accept, 0, len(accepts))
	for i, a := range accepts {
		add := func(field, format string, args ...any) {
			problems = append(problems, Problem{Message: fmt.Sprintf(format, args...), accept: i + 1, field: field})
		}
		a.Term = pyStrLower(strings.TrimFunc(a.Term, pyIsSpace))
		a.Reason = strings.TrimFunc(a.Reason, pyIsSpace)
		a.Index = i
		if a.Term == "" {
			add("term", "accept %d: term is empty; name the word the acceptance covers", i+1)
		}
		if a.Reason == "" {
			add("reason", "accept %d: reason is required; say why the occurrence is acceptable", i+1)
		}
		switch {
		case strings.TrimSpace(a.Path) == "":
			add("path", "accept %d: path is empty; name the file the acceptance covers", i+1)
		case hasDotDotSegment(a.Path):
			add("path", "accept %d: path %q must be repository-relative; it leaves the root", i+1, a.Path)
		default:
			g, ok := pathglob.New(a.Path)
			if !ok || pathglob.Malformed(a.Path) {
				add("path", "accept %d: path glob %q is malformed", i+1, a.Path)
			}
			a.glob = g
		}
		if a.Area != "" && !known[a.Area] {
			add("area", "accept %d: area %q names no area", i+1, a.Area)
		}
		out = append(out, a)
	}
	return out, problems
}

// hasDotDotSegment reports whether a path names a ".." segment, which either leaves the root or never matches
// a leak path, which is always clean and repository-relative.
func hasDotDotSegment(p string) bool {
	for seg := range strings.SplitSeq(strings.TrimSpace(p), "/") {
		if seg == ".." {
			return true
		}
	}
	return false
}

// acceptLine is the 1-based line of the n-th [[accept]] table's header in the lines of an areas file, or of the
// line assigning field in it when it has one; 0 when the table is not found.
func acceptLine(lines []string, n int, field string) int {
	headers := tableHeaders(lines)
	start, seen := -1, 0
	for i, h := range headers {
		if h.ok && h.name == "accept" {
			seen++
			if seen == n {
				start = i
				break
			}
		}
	}
	if start < 0 {
		return 0
	}
	for i := start + 1; field != "" && i < len(lines) && !headers[i].ok; i++ {
		if key, ok := lineKey(lines[i]); ok && key == field {
			return i + 1
		}
	}
	return start + 1
}
