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

// replaceBlock is the script that removes all of a and adds all of b.
func replaceBlock(a, b []string) []diffOp {
	var ops []diffOp
	for _, l := range a {
		ops = append(ops, diffOp{'-', l})
	}
	for _, l := range b {
		ops = append(ops, diffOp{'+', l})
	}
	return ops
}

// lcsTable holds, at [i][j], the length of the longest common subsequence of a[i:] and b[j:].
func lcsTable(a, b []string) [][]int32 {
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
	return lcs
}

func middleScript(a, b []string) []diffOp {
	if len(a) == 0 || len(b) == 0 || len(a)*len(b) > maxDiffCells {
		return replaceBlock(a, b)
	}
	var ops []diffOp
	n, m := len(a), len(b)
	lcs := lcsTable(a, b)
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
	at := opPositions(ops)
	i := 0
	for i < len(ops) {
		for i < len(ops) && ops[i].kind == ' ' {
			i++
		}
		if i == len(ops) {
			break
		}
		start := max(i-diffContext, 0)
		end := hunkEnd(ops, i)
		writeHunk(&sb, ops[start:end], at[start].a, at[start].b, len(al), len(bl), aNL, bNL)
		i = end
	}
	return sb.String()
}

type opPos struct{ a, b int }

// opPositions gives the 0-based positions in a and b before each op, and after the last.
func opPositions(ops []diffOp) []opPos {
	at := make([]opPos, len(ops)+1)
	for i, op := range ops {
		at[i+1] = at[i]
		if op.kind != '+' {
			at[i+1].a++
		}
		if op.kind != '-' {
			at[i+1].b++
		}
	}
	return at
}

// hunkEnd is where the hunk that holds the change at ops[from] stops: it runs through changes and
// gaps of at most 2*context unchanged lines, then takes one more context.
func hunkEnd(ops []diffOp, from int) int {
	end := from
	for end < len(ops) {
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
		return min(j+diffContext, len(ops))
	}
	return end
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

// patchParser is the line-by-line state of ParsePatch.
type patchParser struct {
	files   []PatchFile
	cur     *PatchFile
	hunk    *Hunk
	pending PatchFile
}

func (p *patchParser) flush() {
	if p.hunk != nil && p.cur != nil {
		p.cur.Hunks = append(p.cur.Hunks, *p.hunk)
	}
	p.hunk = nil
}

// header records a `# item`, `# path` or `# digest` line and says whether line was one.
func (p *patchParser) header(line string) bool {
	switch {
	case strings.HasPrefix(line, "# item: "):
		p.pending.Item = strings.TrimPrefix(line, "# item: ")
	case strings.HasPrefix(line, "# path: "):
		p.pending.Path = strings.TrimPrefix(line, "# path: ")
	case strings.HasPrefix(line, "# digest: "):
		p.pending.Digest = strings.TrimPrefix(line, "# digest: ")
	default:
		return false
	}
	return true
}

func (p *patchParser) feed(line string) error {
	if p.header(line) {
		return nil
	}
	switch {
	case strings.HasPrefix(line, "# ai-rulez review fix"):
		p.flush()
		p.cur = nil
		p.pending = PatchFile{}
	case strings.HasPrefix(line, "--- a/"):
		p.flush()
		if path := strings.TrimPrefix(line, "--- a/"); p.pending.Path != "" && path != p.pending.Path {
			return oops.Errorf("the diff is for %q but the header says %q", path, p.pending.Path)
		}
		p.files = append(p.files, p.pending)
		p.cur = &p.files[len(p.files)-1]
		p.pending = PatchFile{}
	case strings.HasPrefix(line, "+++ b/"):
	case strings.HasPrefix(line, "@@ "):
		p.flush()
		if p.cur == nil {
			return oops.Errorf("a hunk before any file header")
		}
		h, err := parseHunkHeader(line)
		if err != nil {
			return err
		}
		p.hunk = &h
	case p.hunk != nil && line != "" && strings.ContainsRune(" +-\\", rune(line[0])):
		p.hunk.Lines = append(p.hunk.Lines, line)
	case strings.HasPrefix(line, "#"), line == "":
	default:
		return oops.Errorf("unexpected line in the patch: %q", line)
	}
	return nil
}

// ParsePatch reads the patch `review fix` writes: per file, `# item`, `# path` and `# digest`
// header lines (anything else starting with # is ignored), then a unified diff.
func ParsePatch(text string) ([]PatchFile, error) {
	var p patchParser
	for _, line := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
		if err := p.feed(line); err != nil {
			return nil, err
		}
	}
	p.flush()
	files := p.files
	for i := range files {
		if files[i].Path == "" || files[i].Digest == "" {
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

// applyHunk applies one hunk at lines[pos:], appending to out. It returns the new position and out,
// and whether a "No newline" marker followed a context or added line.
func applyHunk(lines, out []string, pos int, h *Hunk, hi int) (newPos int, newOut []string, markerNew bool, err error) {
	from := h.OldStart - 1
	if h.OldLines == 0 {
		from = h.OldStart
	}
	if from < pos || from > len(lines) {
		return 0, nil, false, oops.Errorf("hunk %d does not fit the file", hi+1)
	}
	out = append(out, lines[pos:from]...)
	pos = from
	for li, l := range h.Lines {
		kind, body := l[0], l[1:]
		switch kind {
		case ' ', '-':
			if pos >= len(lines) || lines[pos] != body {
				return 0, nil, false, oops.Errorf("hunk %d does not match the file at line %d: the file changed since the patch was made", hi+1, pos+1)
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
	return pos, out, markerNew, nil
}

// ApplyHunks applies hunks to text. Every context and removed line must match exactly where
// the hunk says; there is no fuzz, so a patch made for other text is refused.
func ApplyHunks(text string, hunks []Hunk) (string, error) {
	lines, endsNL := splitLines(text)
	var out []string
	pos := 0
	markerNew := false
	for hi := range hunks {
		var marker bool
		var err error
		pos, out, marker, err = applyHunk(lines, out, pos, &hunks[hi], hi)
		if err != nil {
			return "", err
		}
		markerNew = markerNew || marker
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
