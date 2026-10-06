package review

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/samber/oops"
)

const (
	diffContext = 3
	// maxDiffCells bounds the line-diff table; a larger change is written as one replaced block.
	maxDiffCells = 4_000_000
	noNewline    = `\ No newline at end of file`
)

type diffOp struct {
	kind byte // ' ', '-', '+'
	line string
}

// splitLines splits text into lines without their newline; endsNL says the text ended with one.
func splitLines(text string) (lines []string, endsNL bool) {
	if text == "" {
		return nil, true
	}
	endsNL = strings.HasSuffix(text, "\n")
	lines = strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	return lines, endsNL
}

// editScript is the shortest line edit from a to b: common prefix and suffix are trimmed and a
// longest-common-subsequence table covers the middle.
func editScript(a, b []string) []diffOp {
	pre := 0
	for pre < len(a) && pre < len(b) && a[pre] == b[pre] {
		pre++
	}
	suf := 0
	for suf < len(a)-pre && suf < len(b)-pre && a[len(a)-1-suf] == b[len(b)-1-suf] {
		suf++
	}
	var ops []diffOp
	for _, l := range a[:pre] {
		ops = append(ops, diffOp{' ', l})
	}
	ma, mb := a[pre:len(a)-suf], b[pre:len(b)-suf]
	ops = append(ops, middleScript(ma, mb)...)
	for _, l := range a[len(a)-suf:] {
		ops = append(ops, diffOp{' ', l})
	}
	return ops
}

func middleScript(a, b []string) []diffOp {
	var ops []diffOp
	if len(a) == 0 || len(b) == 0 || len(a)*len(b) > maxDiffCells {
		for _, l := range a {
			ops = append(ops, diffOp{'-', l})
		}
		for _, l := range b {
			ops = append(ops, diffOp{'+', l})
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
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			ops = append(ops, diffOp{' ', a[i]})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			ops = append(ops, diffOp{'-', a[i]})
			i++
		default:
			ops = append(ops, diffOp{'+', b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, diffOp{'-', a[i]})
	}
	for ; j < m; j++ {
		ops = append(ops, diffOp{'+', b[j]})
	}
	return ops
}

// UnifiedDiff renders the change from a to b as a unified diff of the file at path (a/ and b/
// prefixed, three lines of context). It returns "" when the texts are equal.
func UnifiedDiff(path, a, b string) string {
	if a == b {
		return ""
	}
	al, aNL := splitLines(a)
	bl, bNL := splitLines(b)
	ops := editScript(al, bl)
	if last := len(ops) - 1; last >= 0 && allContext(ops) {
		// The texts differ only in the newline at the end: the last line is the change.
		ops = append(ops[:last:last], diffOp{'-', ops[last].line}, diffOp{'+', ops[last].line})
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "--- a/%s\n+++ b/%s\n", path, path)
	// Positions in a and b (0-based) before each op.
	type pos struct{ a, b int }
	at := make([]pos, len(ops)+1)
	for i, op := range ops {
		at[i+1] = at[i]
		if op.kind != '+' {
			at[i+1].a++
		}
		if op.kind != '-' {
			at[i+1].b++
		}
	}
	i := 0
	for i < len(ops) {
		for i < len(ops) && ops[i].kind == ' ' {
			i++
		}
		if i == len(ops) {
			break
		}
		start := max(i-diffContext, 0)
		end := i
		for end < len(ops) {
			// Extend the hunk through changes and gaps of at most 2*context unchanged lines.
			j := end
			for j < len(ops) && ops[j].kind != ' ' {
				j++
			}
			k := j
			for k < len(ops) && ops[k].kind == ' ' {
				k++
			}
			if k < len(ops) && k-j <= 2*diffContext {
				end = k
				continue
			}
			end = min(j+diffContext, len(ops))
			break
		}
		writeHunk(&sb, ops[start:end], at[start].a, at[start].b, len(al), len(bl), aNL, bNL)
		i = end
	}
	return sb.String()
}

func allContext(ops []diffOp) bool {
	for _, op := range ops {
		if op.kind != ' ' {
			return false
		}
	}
	return true
}

func writeHunk(sb *strings.Builder, ops []diffOp, aStart, bStart, aTotal, bTotal int, aNL, bNL bool) {
	aN, bN := 0, 0
	for _, op := range ops {
		if op.kind != '+' {
			aN++
		}
		if op.kind != '-' {
			bN++
		}
	}
	aFrom, bFrom := aStart+1, bStart+1
	if aN == 0 {
		aFrom = aStart
	}
	if bN == 0 {
		bFrom = bStart
	}
	fmt.Fprintf(sb, "@@ -%d,%d +%d,%d @@\n", aFrom, aN, bFrom, bN)
	aPos, bPos := aStart, bStart
	for _, op := range ops {
		sb.WriteByte(op.kind)
		sb.WriteString(op.line)
		sb.WriteString("\n")
		if op.kind != '+' {
			aPos++
		}
		if op.kind != '-' {
			bPos++
		}
		lastA := op.kind != '+' && aPos == aTotal && !aNL
		lastB := op.kind != '-' && bPos == bTotal && !bNL
		if lastA || lastB {
			sb.WriteString(noNewline + "\n")
		}
	}
}

// Hunk is one @@ block of a patch.
type Hunk struct {
	OldStart, OldLines int
	Lines              []string // each with its ' ', '-', '+' or '\' prefix
}

// PatchFile is the patch of one item.
type PatchFile struct {
	Item   string
	Path   string
	Digest string
	Hunks  []Hunk
}

// ParsePatch reads the patch `review fix` writes: per file, `# item`, `# path` and `# digest`
// header lines (anything else starting with # is ignored), then a unified diff.
func ParsePatch(text string) ([]PatchFile, error) {
	var files []PatchFile
	var cur *PatchFile
	var hunk *Hunk
	flush := func() {
		if hunk != nil && cur != nil {
			cur.Hunks = append(cur.Hunks, *hunk)
		}
		hunk = nil
	}
	var pending PatchFile
	for _, line := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
		switch {
		case strings.HasPrefix(line, "# ai-rulez review fix"):
			flush()
			cur = nil
			pending = PatchFile{}
		case strings.HasPrefix(line, "# item: "):
			pending.Item = strings.TrimPrefix(line, "# item: ")
		case strings.HasPrefix(line, "# path: "):
			pending.Path = strings.TrimPrefix(line, "# path: ")
		case strings.HasPrefix(line, "# digest: "):
			pending.Digest = strings.TrimPrefix(line, "# digest: ")
		case strings.HasPrefix(line, "--- a/"):
			flush()
			if p := strings.TrimPrefix(line, "--- a/"); pending.Path != "" && p != pending.Path {
				return nil, oops.Errorf("the diff is for %q but the header says %q", p, pending.Path)
			}
			files = append(files, pending)
			cur = &files[len(files)-1]
			pending = PatchFile{}
		case strings.HasPrefix(line, "+++ b/"):
		case strings.HasPrefix(line, "@@ "):
			flush()
			if cur == nil {
				return nil, oops.Errorf("a hunk before any file header")
			}
			h, err := parseHunkHeader(line)
			if err != nil {
				return nil, err
			}
			hunk = &h
		case hunk != nil && len(line) > 0 && strings.ContainsRune(" +-\\", rune(line[0])):
			hunk.Lines = append(hunk.Lines, line)
		case strings.HasPrefix(line, "#"), line == "":
		default:
			return nil, oops.Errorf("unexpected line in the patch: %q", line)
		}
	}
	flush()
	for _, f := range files {
		if f.Path == "" || f.Digest == "" {
			return nil, oops.Errorf("a patch file entry has no path or digest header: it was not written by `ai-rulez review fix`")
		}
	}
	if len(files) == 0 {
		return nil, oops.Errorf("the patch holds no file")
	}
	return files, nil
}

func parseHunkHeader(line string) (Hunk, error) {
	var h Hunk
	fields := strings.Fields(line)
	if len(fields) < 3 || !strings.HasPrefix(fields[1], "-") {
		return h, oops.Errorf("bad hunk header %q", line)
	}
	start, lines, _ := strings.Cut(strings.TrimPrefix(fields[1], "-"), ",")
	s, err1 := strconv.Atoi(start)
	n := 1
	var err2 error
	if lines != "" {
		n, err2 = strconv.Atoi(lines)
	}
	if err1 != nil || err2 != nil || s < 0 || n < 0 {
		return h, oops.Errorf("bad hunk header %q", line)
	}
	h.OldStart, h.OldLines = s, n
	return h, nil
}

// ApplyHunks applies hunks to text. Every context and removed line must match exactly where
// the hunk says; there is no fuzz, so a patch made for other text is refused.
func ApplyHunks(text string, hunks []Hunk) (string, error) {
	lines, endsNL := splitLines(text)
	var out []string
	pos := 0
	markerNew := false
	for hi, h := range hunks {
		from := h.OldStart - 1
		if h.OldLines == 0 {
			from = h.OldStart
		}
		if from < pos || from > len(lines) {
			return "", oops.Errorf("hunk %d does not fit the file", hi+1)
		}
		out = append(out, lines[pos:from]...)
		pos = from
		for li, l := range h.Lines {
			kind, body := l[0], l[1:]
			switch kind {
			case ' ', '-':
				if pos >= len(lines) || lines[pos] != body {
					return "", oops.Errorf("hunk %d does not match the file at line %d: the file changed since the patch was made", hi+1, pos+1)
				}
				pos++
				if kind == ' ' {
					out = append(out, body)
				}
			case '+':
				out = append(out, body)
			case '\\':
				// "No newline at end of file" applies to the line before it.
				if li > 0 {
					prev := h.Lines[li-1][0]
					if prev == ' ' || prev == '+' {
						markerNew = true
					}
				}
			}
		}
	}
	// When the hunks reach the end of the file they decide whether it ends with a newline;
	// otherwise the untouched tail keeps the original's.
	newEndsNL := endsNL
	if pos >= len(lines) && len(hunks) > 0 {
		newEndsNL = !markerNew
	}
	out = append(out, lines[pos:]...)
	if len(out) == 0 {
		return "", nil
	}
	res := strings.Join(out, "\n")
	if newEndsNL {
		res += "\n"
	}
	return res, nil
}
