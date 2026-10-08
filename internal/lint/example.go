package lint

import (
	"regexp"
	"strings"
)

// Example regions let documentation show a risky command without tripping the
// rules that match command shapes (curl | sh, reads of ~/.ssh, URLs outside
// an allow-list). A region is

//   - a fenced code block whose info string contains the word "example"
//     (```bash example), or that directly follows an <!-- ai-rulez-example -->
//     comment, or
//   - any line of a file matching [lint] example_paths.
//
// Only rules registered with MarkExampleAware skip findings inside a region.
// Secrets, hidden characters, injection phrases and comment instructions are not
// command-shaped and are never skipped. Markers inside imported content
// (scan_imports) are ignored: imported text cannot vouch for itself.

// exampleMarker is the comment that marks the next fenced block as an example.
const exampleMarker = "ai-rulez-example"

var exampleInfoRe = regexp.MustCompile(`(?i)(^|[\s,;:{="'])examples?($|[\s,;:}"'])`)

// exampleAware lists the rules that honor example regions.
var exampleAware = map[string]bool{
	CodeShellExec: true, CodeShellAccess: true, CodeOutboundHost: true,
}

// MarkExampleAware registers rules (by code) whose findings are dropped inside
// an example region. Call it from an init function next to the rule's
// registration; a command-shaped rule should do so, a content rule should not.
func MarkExampleAware(codes ...string) {
	for _, c := range codes {
		exampleAware[c] = true
	}
}

// exampleLines returns the 1-based lines of raw that sit inside an example
// fence, including the fence lines themselves.
func exampleLines(d doc) map[int]bool {
	var out map[int]bool
	var open string
	start := 0
	for i, line := range d.lines {
		m := fenceRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		run, info := m[1], strings.TrimSpace(m[2])
		if open == "" {
			if run[0] == '`' && strings.Contains(info, "`") {
				continue // an inline span, not a fence
			}
			open, start = run, i
			if isExampleFence(d, i, info) {
				continue
			}
			start = -1
			continue
		}
		if run[0] == open[0] && len(run) >= len(open) && info == "" {
			if start >= 0 {
				if out == nil {
					out = map[int]bool{}
				}
				for l := start; l <= i; l++ {
					out[l+1] = true
				}
			}
			open = ""
		}
	}
	return out
}

// isExampleFence reports whether the fence opened at line idx is an example:
// by its info string or by a marker comment on the previous non-blank line.
func isExampleFence(d doc, idx int, info string) bool {
	if exampleInfoRe.MatchString(info) {
		return true
	}
	for j := idx - 1; j >= 0; j-- {
		if strings.TrimSpace(d.lines[j]) == "" {
			continue
		}
		return strings.Contains(d.lines[j], exampleMarker)
	}
	return false
}

// inExample reports whether a finding at (abs, line) lies in an example
// region. Rules that scan line by line may call it to skip work early.
func (r *runner) inExample(abs string, line int) bool {
	if r.examplePath(abs) {
		return true
	}
	if r.forceSev != "" {
		return false
	}
	lines, ok := r.exampleCache[abs]
	if !ok {
		if d, have := r.docs[abs]; have {
			lines = exampleLines(d)
		}
		if r.exampleCache == nil {
			r.exampleCache = map[string]map[int]bool{}
		}
		r.exampleCache[abs] = lines
	}
	return lines[line]
}

// examplePath reports whether abs matches a configured example path.
func (r *runner) examplePath(abs string) bool {
	if len(r.exampleGlobs) == 0 {
		return false
	}
	cands := []string{r.rel(abs)}
	cands = append(cands, r.configRel(abs))
	for _, c := range cands {
		for _, g := range r.exampleGlobs {
			if c != "" && g.match(c) {
				return true
			}
		}
	}
	return false
}
