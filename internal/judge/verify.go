package judge

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Verifier enforces the evidence contract mechanically. A finding is kept only when its quote is a
// whitespace-normalized substring of the text that was actually sent (after masking), at the cited
// lines of a file of the unit. The model is never trusted to have quoted correctly.
type Verifier struct {
	unit  Unit
	conv  map[string]Convention
	facts map[string]bool
	// Category is the stage being verified; it fixes the code enum and the severity rules.
	Category Category
	// Rubric holds the criterion ids that may appear as rubric_id.
	Rubric map[string]bool
}

// NewVerifier returns a verifier for one unit and stage. conv are the conventions the stage was
// shown; facts the static fact ids.
func NewVerifier(u Unit, cat Category, conv []Convention, facts []Fact, rubricIDs []string) *Verifier {
	v := &Verifier{unit: u, Category: cat, conv: map[string]Convention{}, facts: map[string]bool{}, Rubric: map[string]bool{}}
	for i := range conv {
		v.conv[conv[i].ID] = conv[i]
	}
	for _, f := range facts {
		v.facts[f.ID] = true
	}
	for _, id := range rubricIDs {
		v.Rubric[id] = true
	}
	return v
}

// Problem is one reason a candidate failed verification.
type Problem struct {
	CandidateID string `json:"candidate_id"`
	Field       string `json:"field"`
	Message     string `json:"message"`
}

func (p Problem) String() string { return p.CandidateID + " " + p.Field + ": " + p.Message }

// Verified is a candidate that passed, as sanitized by the verifier.
type Verified struct {
	Candidate Candidate
	// ChainStepsDropped counts chain steps removed because their quote did not verify.
	ChainStepsDropped int
	// AlsoAtDropped counts also_at entries removed for the same reason.
	AlsoAtDropped int
	// Capped lists the deterministic severity caps applied.
	Capped []string
}

var linePrefixRe = regexp.MustCompile(`(?m)^\s*L\d+\|\s?`)

// Check verifies one candidate. It returns the sanitized candidate when there are no problems.
func (v *Verifier) Check(c Candidate) (Verified, []Problem) {
	var probs []Problem
	fail := func(field, format string, args ...any) {
		probs = append(probs, Problem{CandidateID: c.ID, Field: field, Message: fmt.Sprintf(format, args...)})
	}
	if strings.TrimSpace(c.ID) == "" {
		fail("id", "empty")
	}
	info, known := LookupCode(c.Code)
	switch {
	case !known:
		fail("code", "%q is not a finding code", c.Code)
	case info.Category != v.Category && (c.Code != codeRubricFailure || len(v.Rubric) == 0):
		fail("code", "%s belongs to the %s reviewer; this stage reports only %s codes", c.Code, info.Category, v.Category)
	}
	if !c.Severity.Valid() {
		fail("severity", "%q is not critical, high, medium, low or info", c.Severity)
	}
	if c.Confidence.Rank() < 0 {
		fail("confidence", "%q is not high, medium or low", c.Confidence)
	}
	if c.Verdict != VerdictConfirmed && c.Verdict != VerdictContextualRisk {
		fail("verdict", "%q is not confirmed or contextual_risk", c.Verdict)
	}
	if msg := v.checkQuote(c.Location, c.Quote); msg != "" {
		fail("quote", "%s", msg)
	}
	if len(probs) > 0 {
		return Verified{}, probs
	}
	out := Verified{Candidate: c}
	out.Candidate.Chain, out.ChainStepsDropped = v.cleanChain(c.Chain)
	out.Candidate.AlsoAt, out.AlsoAtDropped = v.cleanAlsoAt(c.AlsoAt)
	out.Candidate.StaticRefs = v.cleanFacts(c.StaticRefs)
	if v.Category == CategoryGovernance || c.Code == codeRubricFailure {
		probs = append(probs, v.checkGovernance(&out.Candidate)...)
		if len(probs) > 0 {
			return Verified{}, probs
		}
	}
	catOfCode, _ := CategoryOfCode(c.Code)
	out.Capped = capSeverity(&out.Candidate, catOfCode)
	return out, nil
}

// checkQuote returns "" when quote is a verbatim (whitespace-normalized) copy of text at loc.
func (v *Verifier) checkQuote(loc Location, quote string) string {
	f, ok := v.unit.fileByPath(loc.Path)
	switch {
	case !ok || len(f.Lines) == 0:
		return fmt.Sprintf("path %q is not a file that was sent", loc.Path)
	case loc.StartLine < 1 || loc.EndLine < loc.StartLine || loc.EndLine > len(f.Lines):
		return fmt.Sprintf("lines %d-%d are outside %s (1-%d)", loc.StartLine, loc.EndLine, loc.Path, len(f.Lines))
	case loc.EndLine-loc.StartLine >= maxLocationLines:
		return fmt.Sprintf("the location spans %d lines; at most %d", loc.EndLine-loc.StartLine+1, maxLocationLines)
	case strings.TrimSpace(quote) == "":
		return "the quote is empty"
	case utf8.RuneCountInString(quote) > maxQuoteChars:
		return fmt.Sprintf("the quote is %d characters; at most %d", utf8.RuneCountInString(quote), maxQuoteChars)
	}
	hay := NormalizeSpace(strings.Join(f.Lines[loc.StartLine-1:loc.EndLine], "\n"))
	needle := NormalizeSpace(quote)
	if strings.Contains(hay, needle) {
		return ""
	}
	if linePrefixRe.MatchString(quote) {
		return "the quote contains the L<n>| line prefixes; copy only the file text"
	}
	return fmt.Sprintf("the quote is not a verbatim copy of %s lines %d-%d", loc.Path, loc.StartLine, loc.EndLine)
}

func (v *Verifier) cleanChain(steps []Step) (kept []Step, dropped int) {
	if len(steps) == 0 {
		return nil, 0
	}
	for _, s := range steps {
		if len(kept) >= maxChainSteps {
			dropped++
			continue
		}
		if !validStepRole(s.Role) || v.checkQuote(s.Location, s.Quote) != "" {
			dropped++
			continue
		}
		kept = append(kept, s)
	}
	if len(kept) < minChainSteps {
		return nil, dropped + len(kept)
	}
	return kept, dropped
}

func validStepRole(r string) bool {
	switch r {
	case "source", "transform", "trigger", "sink", "scope":
		return true
	}
	return false
}

func (v *Verifier) cleanAlsoAt(in []AlsoAt) (kept []AlsoAt, dropped int) {
	for _, a := range in {
		if v.checkQuote(a.Location, a.Quote) != "" {
			dropped++
			continue
		}
		kept = append(kept, a)
	}
	return kept, dropped
}

func (v *Verifier) cleanFacts(refs []string) []string {
	var kept []string
	for _, r := range refs {
		if v.facts[r] {
			kept = append(kept, r)
		}
	}
	return kept
}

// needsConventionQuote reports whether a governance code rests on two quotes: the file and a convention.
func needsConventionQuote(code string) bool {
	switch code {
	case "JR-G01", "JR-G02", "JR-G03", "JR-G05", "JR-G07":
		return true
	}
	return false
}

var bindingWords = regexp.MustCompile(`(?i)\b(must|must not|never|always|required|requires|forbidden|shall|shall not|not allowed|prohibited|do not|don't|may not)\b`)

func (v *Verifier) checkGovernance(c *Candidate) []Problem {
	var probs []Problem
	fail := func(field, format string, args ...any) {
		probs = append(probs, Problem{CandidateID: c.ID, Field: field, Message: fmt.Sprintf(format, args...)})
	}
	switch {
	case c.Code == codeRubricFailure:
		if c.RubricID == nil || !v.Rubric[*c.RubricID] {
			fail("rubric_id", "a rubric_failure names a criterion of the loaded rubric")
		}
	case c.Convention == nil && needsConventionQuote(c.Code):
		fail("convention", "%s needs the convention line it rests on (id, line, quote)", c.Code)
	}
	if c.Convention != nil {
		if msg := v.checkConvention(*c.Convention); msg != "" {
			fail("convention", "%s", msg)
		}
	}
	return probs
}

func (v *Verifier) checkConvention(cr ConventionRef) string {
	conv, ok := v.conv[cr.ID]
	switch {
	case !ok:
		return fmt.Sprintf("%q is not an attached convention", cr.ID)
	case len(conv.Lines) == 0:
		return fmt.Sprintf("%s was sent as an index only; it cannot be quoted", cr.ID)
	case cr.Line < 1 || cr.Line > len(conv.Lines):
		return fmt.Sprintf("line %d is outside %s (1-%d)", cr.Line, cr.ID, len(conv.Lines))
	case strings.TrimSpace(cr.Quote) == "":
		return "the convention quote is empty"
	case utf8.RuneCountInString(cr.Quote) > maxQuoteChars:
		return "the convention quote is too long"
	}
	lo, hi := max(cr.Line-1, 1), min(cr.Line+1, len(conv.Lines))
	if !strings.Contains(NormalizeSpace(strings.Join(conv.Lines[lo-1:hi], "\n")), NormalizeSpace(cr.Quote)) {
		return fmt.Sprintf("the quote is not a verbatim copy of %s line %d", cr.ID, cr.Line)
	}
	return ""
}

// capSeverity applies the deterministic severity ceilings of the role prompts and returns what it did.
func capSeverity(c *Candidate, cat Category) []string {
	var notes []string
	limit := func(max Severity, why string) {
		if c.Severity.Rank() > max.Rank() {
			notes = append(notes, fmt.Sprintf("%s -> %s: %s", c.Severity, max, why))
			c.Severity = max
		}
	}
	switch cat {
	case CategorySecurity:
		if c.Verdict == VerdictContextualRisk && len(c.Chain) == 0 {
			limit(SeverityMedium, "a contextual risk with no quoted chain")
		}
	case CategoryQuality:
		if c.Code == "JR-Q03" {
			limit(SeverityHigh, "quality findings stop at high for contradictions")
		} else {
			limit(SeverityMedium, "quality findings stop at medium")
		}
	case CategoryGovernance:
		if c.Convention != nil && !bindingWords.MatchString(c.Convention.Quote) && c.Code != codeRubricFailure {
			limit(SeverityLow, "the convention line is guidance, not binding")
		}
	}
	return notes
}

// FindingID is the stable id of a finding: it survives line moves and reruns.
func FindingID(code, path, quote string) string {
	sum := sha256.Sum256([]byte(code + "|" + path + "|" + NormalizeSpace(quote)))
	return hex.EncodeToString(sum[:])[:12]
}

// Window returns the lines around loc (radius lines each way) with their numbers, for the adjudicator.
func (u Unit) Window(loc Location, radius int) (text string, start, end int, ok bool) {
	f, found := u.fileByPath(loc.Path)
	if !found || len(f.Lines) == 0 || loc.StartLine < 1 || loc.EndLine > len(f.Lines) || loc.EndLine < loc.StartLine {
		return "", 0, 0, false
	}
	start, end = max(loc.StartLine-radius, 1), min(loc.EndLine+radius, len(f.Lines))
	var b strings.Builder
	for i := start; i <= end; i++ {
		fmt.Fprintf(&b, "L%d| %s\n", i, f.Lines[i-1])
	}
	return b.String(), start, end, true
}

// VerbatimIn reports whether counter appears (whitespace-normalized) in the given window texts.
func VerbatimIn(counter string, windows []string) bool {
	needle := NormalizeSpace(counter)
	if needle == "" {
		return false
	}
	for _, w := range windows {
		if strings.Contains(NormalizeSpace(linePrefixRe.ReplaceAllString(w, "")), needle) {
			return true
		}
	}
	return false
}

// FingerprintOf is the full-length hash behind a finding's stable id, for SARIF partial fingerprints.
func FingerprintOf(code, path, quote string) string {
	sum := sha256.Sum256([]byte(code + "|" + path + "|" + NormalizeSpace(quote)))
	return hex.EncodeToString(sum[:])
}
