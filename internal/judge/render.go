package judge

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
)

const (
	// DefaultInputTokens is the content budget of one reviewer call.
	DefaultInputTokens = 24000
	// chunkOverlapLines is how many lines two chunks of one file share, so a span at a boundary is whole in one of them.
	chunkOverlapLines = 40
	// maxChunksPerUnit bounds the calls one category spends on a unit.
	maxChunksPerUnit = 8
	// maxDigestLines bounds the capability digest of a chunked unit.
	maxDigestLines = 80
	// maxSiblings bounds the sibling index of the quality reviewer.
	maxSiblings = 40
	// linePrefixTokens approximates the tokens the `L<n>| ` prefix adds to a line.
	linePrefixTokens = 3
)

// Segment is a line range (1-based, inclusive) of one file of a unit.
type Segment struct {
	File       int
	Start, End int
}

// Chunk is the part of a unit one call sees.
type Chunk struct {
	Index    int
	Segments []Segment
}

// PlanChunks splits the sent text of a unit into chunks of at most capTokens tokens. Whole files are
// packed together; a file too large for one chunk is cut into windows that overlap by 40 lines. The
// spans that no chunk can hold (past the chunk limit) are returned as not reviewed.
func PlanChunks(u Unit, capTokens int) (chunks []Chunk, missed []NotReviewed) {
	capTokens = max(capTokens, 2000)
	cur := Chunk{}
	used := 0
	flush := func() {
		if len(cur.Segments) > 0 {
			cur.Index = len(chunks)
			chunks = append(chunks, cur)
		}
		cur, used = Chunk{}, 0
	}
	for fi := range u.Files {
		f := &u.Files[fi]
		if len(f.Lines) == 0 {
			continue
		}
		cost, total := lineCosts(f.Lines)
		whole := Segment{File: fi, Start: 1, End: len(f.Lines)}
		if used+total <= capTokens {
			cur.Segments = append(cur.Segments, whole)
			used += total
			continue
		}
		flush()
		if total <= capTokens {
			cur.Segments, used = append(cur.Segments, whole), total
			continue
		}
		for _, w := range overlappingWindows(cost, capTokens) {
			cur.Segments = append(cur.Segments, Segment{File: fi, Start: w[0], End: w[1]})
			flush()
		}
	}
	flush()
	return limitChunks(u, chunks)
}

// lineCosts estimates the tokens of each numbered line and their sum.
func lineCosts(lines []string) (cost []int, total int) {
	cost = make([]int, len(lines))
	for i, l := range lines {
		cost[i] = tokens.Estimate(l) + linePrefixTokens
		total += cost[i]
	}
	return cost, total
}

// overlappingWindows cuts the lines of a file too big for one chunk into line ranges (1-based,
// inclusive) of at most capTokens, each starting chunkOverlapLines before the end of the previous.
func overlappingWindows(cost []int, capTokens int) [][2]int {
	var out [][2]int
	n := len(cost)
	for start := 1; start <= n; {
		end, sum := start, 0
		for end <= n && sum+cost[end-1] <= capTokens {
			sum += cost[end-1]
			end++
		}
		end-- // the last line that fit
		if end < start {
			end = start // a single line larger than the budget is sent alone
		}
		out = append(out, [2]int{start, end})
		if end >= n {
			break
		}
		start = max(end-chunkOverlapLines+1, start+1)
	}
	return out
}

// limitChunks keeps the first maxChunksPerUnit chunks and reports the spans of the rest.
func limitChunks(u Unit, chunks []Chunk) (kept []Chunk, missed []NotReviewed) {
	if len(chunks) <= maxChunksPerUnit {
		return chunks, nil
	}
	for ci := maxChunksPerUnit; ci < len(chunks); ci++ {
		for _, s := range chunks[ci].Segments {
			missed = append(missed, NotReviewed{Path: u.Files[s.File].Path, StartLine: s.Start, EndLine: s.End, Reason: "beyond the limit of " + itoa(maxChunksPerUnit) + " chunks per unit"})
		}
	}
	return chunks[:maxChunksPerUnit], missed
}

// renderContent renders the untrusted block of one chunk: every file with its numbered lines, the
// files that were not sent with their reason, and, for a chunked unit, the digest of the other chunks.
func renderContent(u Unit, c Chunk, chunkCount int) string {
	var b strings.Builder
	inChunk := map[int][]Segment{}
	for _, s := range c.Segments {
		inChunk[s.File] = append(inChunk[s.File], s)
	}
	for fi, f := range u.Files {
		switch {
		case f.Omitted != "":
			fmt.Fprintf(&b, "=== FILE %s  (kind=%s, NOT SENT: %s)\n", f.Path, f.Role, f.Omitted)
		case len(inChunk[fi]) > 0:
			for _, s := range inChunk[fi] {
				fmt.Fprintf(&b, "=== FILE %s  (kind=%s, lines %d-%d of %d", f.Path, f.Role, s.Start, s.End, len(f.Lines))
				if f.Redacted {
					b.WriteString("; secret-looking text masked")
				}
				b.WriteString(")\n")
				for i := s.Start; i <= s.End; i++ {
					fmt.Fprintf(&b, "L%d| %s\n", i, f.Lines[i-1])
				}
			}
		case len(f.Lines) > 0:
			fmt.Fprintf(&b, "=== FILE %s  (kind=%s, %d lines, in another chunk of this unit)\n", f.Path, f.Role, len(f.Lines))
		}
	}
	if chunkCount > 1 {
		b.WriteString(capabilityDigest(u, c))
	}
	return strings.TrimRight(b.String(), "\n")
}

var capabilityRe = regexp.MustCompile(`(?i)(curl|wget|https?://|fetch\(|requests\.|urllib|socket|nc |scp |rsync|eval|exec|subprocess|os\.system|child_process|\bsh -c|\bbash\b|chmod|sudo|rm -rf|\.aws|\.ssh|credential|token|secret|password|api[_-]?key|\benv\b|before (every|each|any)|do not (tell|mention|show)|silently|ignore (all|previous|prior))`)

// capabilityDigest lists, per file, the lines outside the chunk that touch the network, a shell,
// credentials or an instruction to the agent, deterministically, so a chain whose source and sink
// sit in different chunks stays visible. It is part of the untrusted block: it is file text.
func capabilityDigest(u Unit, c Chunk) string {
	have := map[int][]Segment{}
	for _, s := range c.Segments {
		have[s.File] = append(have[s.File], s)
	}
	covered := func(fi, line int) bool {
		for _, s := range have[fi] {
			if line >= s.Start && line <= s.End {
				return true
			}
		}
		return false
	}
	var b strings.Builder
	n := 0
	for fi, f := range u.Files {
		first := true
		for i, l := range f.Lines {
			if n >= maxDigestLines {
				break
			}
			if covered(fi, i+1) || !capabilityRe.MatchString(l) {
				continue
			}
			if first {
				fmt.Fprintf(&b, "=== DIGEST of %s (capability lines outside this chunk; context only, cite lines only from FILE blocks)\n", f.Path)
				first = false
			}
			fmt.Fprintf(&b, "L%d| %s\n", i+1, truncateRunes(strings.TrimSpace(l), 160))
			n++
		}
	}
	return b.String()
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}

// renderFacts renders the static facts, one JSON object per line.
func renderFacts(facts []Fact) string {
	if len(facts) == 0 {
		return noText
	}
	var b strings.Builder
	for _, f := range facts {
		b.Write(jsonText(f))
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

// The modes of renderConventions.
const (
	convModeFull  = "full"
	convModeIndex = "index"
	convModeOmit  = "omit"
)

// renderConventions renders the attached conventions. mode is convModeFull (governance),
// convModeIndex (the quality reviewer: ids, scope and headings only) or convModeOmit.
func renderConventions(conv []Convention, mode string) string {
	if len(conv) == 0 || mode == convModeOmit {
		return noText
	}
	var b strings.Builder
	for ci := range conv {
		c := &conv[ci]
		fmt.Fprintf(&b, "[convention id=%s  attached=%s", c.ID, c.Why)
		if len(c.Paths) > 0 {
			fmt.Fprintf(&b, "  paths=%s", jsonText(c.Paths))
		}
		fmt.Fprintf(&b, "  sha256=%s]\n", c.SHA256)
		switch {
		case mode == convModeFull && c.Full:
			for i, l := range c.Lines {
				fmt.Fprintf(&b, "L%d| %s\n", i+1, l)
			}
		default:
			for _, l := range c.Index {
				b.WriteString(l + "\n")
			}
			if mode == convModeFull {
				b.WriteString("(index only: too large to send in full)\n")
			}
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// reviewerInput is everything the user message of a reviewer call is made of.
type reviewerInput struct {
	Unit       Unit
	Category   Category
	Chunk      Chunk
	ChunkCount int
	Facts      []Fact
	Conv       []Convention
	ConvMode   string
	Instr      string
	// ExtraConv is text appended to the conventions block (T2 rubric blocks).
	ExtraConv string
	Siblings  string
	Nonce     string
}

func (in reviewerInput) render() string {
	sib := ""
	if in.Category == CategoryQuality {
		sib = "SIBLINGS (quality only, id: description): " + orNone(in.Siblings) + "\n"
	}
	// One pass over the template: text inserted for one placeholder is never scanned for another.
	return strings.NewReplacer(
		"{{unit_id}}", in.Unit.ID,
		"{{kind}}", string(in.Unit.Kind),
		"{{unit_path}}", in.Unit.Path,
		"{{unit_sha256}}", in.Unit.Hash[:16],
		"{{category}}", string(in.Category),
		"{{prompt_version}}", PromptVersion,
		"{{chunk_index}}", itoa(in.Chunk.Index+1),
		"{{chunk_count}}", itoa(in.ChunkCount),
		"{{siblings}}", sib,
		"{{NONCE}}", in.Nonce,
		"{{reviewer_instructions}}", orNone(in.Instr),
		"{{conventions}}", joinNonEmpty(renderConventions(in.Conv, in.ConvMode), in.ExtraConv),
		"{{facts}}", renderFacts(in.Facts),
		"{{content}}", renderContent(in.Unit, in.Chunk, in.ChunkCount),
	).Replace(promptText("50_user_message_template.txt"))
}

func joinNonEmpty(parts ...string) string {
	var out []string
	for _, p := range parts {
		if strings.TrimSpace(p) != "" && p != noText {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return noText
	}
	return strings.Join(out, "\n")
}

func orNone(s string) string {
	if strings.TrimSpace(s) == "" {
		return noText
	}
	return s
}

// deriveNonce makes the fence token of a request: deterministic (so the response cache stays
// effective), unpredictable to whoever wrote the files (it hashes content they cannot see whole in
// advance together with the prompt fingerprint), and absent from every text that is sent.
func deriveNonce(parts []string, texts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	sum := h.Sum(nil)
	for {
		nonce := hex.EncodeToString(sum[:12])
		clash := false
		for _, t := range texts {
			if strings.Contains(t, nonce) {
				clash = true
				break
			}
		}
		if !clash {
			return nonce
		}
		next := sha256.Sum256(sum)
		sum = next[:]
	}
}

// unitTexts returns the file text of a unit, for the nonce clash check.
func unitTexts(u Unit) []string {
	var out []string
	for _, f := range u.Files {
		out = append(out, strings.Join(f.Lines, "\n"))
	}
	return out
}
