package improve

import (
	"bytes"
	"fmt"
	"strings"
)

// contextLines is the unchanged context around each hunk.
const contextLines = 3

// maxLCSCells bounds the line-diff table; past it a file is rewritten whole.
const maxLCSCells = 16 << 20

// UnifiedDiff renders the changes from orig to cand as a patch `git apply`
// accepts. prefix is the repository-relative skill directory (slash separated),
// so the headers name the authored location.
func UnifiedDiff(prefix string, orig, cand *Tree) string {
	seen := map[string]bool{}
	var paths []string
	for _, p := range append(orig.Paths(), cand.Paths()...) {
		if !seen[p] {
			seen[p] = true
			paths = append(paths, p)
		}
	}
	sortStrings(paths)
	var out strings.Builder
	for _, p := range paths {
		before, hadBefore := orig.Files[p]
		after, hasAfter := cand.Files[p]
		if hadBefore && hasAfter && bytes.Equal(before.Data, after.Data) && before.Exec == after.Exec {
			continue
		}
		full := strings.TrimSuffix(prefix, "/") + "/" + p
		out.WriteString(fileDiff(full, before, hadBefore, after, hasAfter))
	}
	return out.String()
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func fileDiff(full string, before Entry, hadBefore bool, after Entry, hasAfter bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "diff --git a/%s b/%s\n", full, full)
	oldName, newName := "a/"+full, "b/"+full
	switch {
	case !hadBefore:
		fmt.Fprintf(&b, "new file mode %s\n", modeOf(after))
		oldName = "/dev/null"
	case !hasAfter:
		fmt.Fprintf(&b, "deleted file mode %s\n", modeOf(before))
		newName = "/dev/null"
	case before.Exec != after.Exec:
		fmt.Fprintf(&b, "old mode %s\nnew mode %s\n", modeOf(before), modeOf(after))
	}
	oldLines, newLines := splitLines(before.Data), splitLines(after.Data)
	if !hadBefore {
		oldLines = nil
	}
	if !hasAfter {
		newLines = nil
	}
	if bytes.Equal(before.Data, after.Data) && hadBefore && hasAfter {
		return b.String() // a mode-only change has no hunks
	}
	fmt.Fprintf(&b, "--- %s\n+++ %s\n", oldName, newName)
	b.WriteString(hunks(oldLines, newLines))
	return b.String()
}

func modeOf(e Entry) string {
	if e.Exec {
		return "100755"
	}
	return "100644"
}

// splitLines splits keeping a marker for a missing final newline.
func splitLines(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	text := string(data)
	lines := strings.SplitAfter(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

type op struct {
	kind byte // ' ', '-', '+'
	text string
}

func hunks(a, b []string) string {
	ops := diffOps(a, b)
	var out strings.Builder
	i := 0
	for i < len(ops) {
		for i < len(ops) && ops[i].kind == ' ' {
			i++
		}
		if i >= len(ops) {
			break
		}
		start := max(i-contextLines, 0)
		stop := hunkEnd(ops, i)
		oldStart, newStart := 1, 1
		for _, o := range ops[:start] {
			if o.kind != '+' {
				oldStart++
			}
			if o.kind != '-' {
				newStart++
			}
		}
		oldN, newN, body := hunkBody(ops[start:stop])
		if oldN == 0 {
			oldStart--
		}
		if newN == 0 {
			newStart--
		}
		fmt.Fprintf(&out, "@@ -%s +%s @@\n%s", rng(oldStart, oldN), rng(newStart, newN), body)
		i = stop
	}
	return out.String()
}

// hunkEnd returns the exclusive end of the hunk whose first change is at ops[first]: the last change of the
// run of changes closer together than twice the context, plus the trailing context.
func hunkEnd(ops []op, first int) int {
	end := first
	lastChange := first
	for end < len(ops) {
		if ops[end].kind != ' ' {
			lastChange = end
		} else if end-lastChange > 2*contextLines {
			break
		}
		end++
	}
	return min(lastChange+contextLines+1, len(ops))
}

// hunkBody renders the lines of one hunk and counts the old and new lines it covers.
func hunkBody(ops []op) (oldN, newN int, body string) {
	var sb strings.Builder
	for _, o := range ops {
		if o.kind != '+' {
			oldN++
		}
		if o.kind != '-' {
			newN++
		}
		sb.WriteByte(o.kind)
		sb.WriteString(o.text)
		if !strings.HasSuffix(o.text, "\n") {
			sb.WriteString("\n\\ No newline at end of file\n")
		}
	}
	return oldN, newN, sb.String()
}

func rng(start, n int) string {
	if n == 1 {
		return fmt.Sprint(start)
	}
	return fmt.Sprintf("%d,%d", start, n)
}

// diffOps is a line diff by longest common subsequence.
func diffOps(a, b []string) []op {
	if (len(a)+1)*(len(b)+1) > maxLCSCells {
		ops := make([]op, 0, len(a)+len(b))
		for _, l := range a {
			ops = append(ops, op{'-', l})
		}
		for _, l := range b {
			ops = append(ops, op{'+', l})
		}
		return ops
	}
	n, m := len(a), len(b)
	lcs := make([][]int32, n+1)
	for i := range lcs {
		lcs[i] = make([]int32, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var ops []op
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			ops = append(ops, op{' ', a[i]})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			ops = append(ops, op{'-', a[i]})
			i++
		default:
			ops = append(ops, op{'+', b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, op{'-', a[i]})
	}
	for ; j < m; j++ {
		ops = append(ops, op{'+', b[j]})
	}
	return ops
}
