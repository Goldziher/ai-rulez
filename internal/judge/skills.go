package judge

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Goldziher/ai-rulez/v5/internal/frontmatter"
)

// Trust tiers of text that reaches a reviewer (docs/judge.md).
const (
	// TierConfigured (T1) is rubric text the owner of the run configured: a skill named by path on
	// the command line, a pinned skill, a prompt extension file.
	TierConfigured = "T1"
	// TierReference (T2) is reference data: an inline or unpinned skill, conventions and static facts.
	TierReference = "T2"
)

// Criterion is one checkable item of a rubric skill.
type Criterion struct {
	// ID is "rubric:<skill>/C1".
	ID    string
	Title string
	Text  string
}

// Rubric is a skill whose `## C1 - title` sections are criteria a unit is judged against.
type Rubric struct {
	Name        string
	Description string
	Path        string
	// SHA256 is the first 16 hex digits of the file's hash; fullSHA256 all of them.
	SHA256     string
	fullSHA256 string
	Tier       string
	Criteria   []Criterion
}

// Pinned reports whether the skill's hash starts with pin (at least 8 hex digits).
func (r Rubric) Pinned(pin string) bool {
	pin = strings.ToLower(strings.TrimSpace(pin))
	return len(pin) >= 8 && strings.HasPrefix(r.fullSHA256, pin)
}

var criterionHeadRe = regexp.MustCompile(`^##\s+(C\d+)\s*(?:[-:\x{2013}\x{2014}]\s*)?(.*)$`)

// ParseRubric reads a rubric skill. name is used when the frontmatter has none.
func ParseRubric(name, path, text, tier string) (Rubric, error) {
	sum := sha256.Sum256([]byte(text))
	full := hex.EncodeToString(sum[:])
	r := Rubric{Name: name, Path: path, SHA256: full[:16], fullSHA256: full, Tier: tier}
	fm := frontmatter.SplitString(text)
	if fm.Closed {
		r.readFrontmatter(fm.Raw)
	}
	if r.Name == "" {
		return r, fmt.Errorf("a rubric skill needs a name")
	}
	r.Criteria = parseCriteria(r.Name, fm.Body)
	if len(r.Criteria) == 0 {
		return r, fmt.Errorf("skill %s has no `## C1 - title` criteria sections, so it is not a rubric", r.Name)
	}
	seen := map[string]bool{}
	for _, c := range r.Criteria {
		if seen[c.ID] {
			return r, fmt.Errorf("skill %s defines criterion %s twice", r.Name, c.ID)
		}
		seen[c.ID] = true
	}
	return r, nil
}

// readFrontmatter takes the name and description from the YAML frontmatter.
func (r *Rubric) readFrontmatter(raw string) {
	var doc map[string]any
	if yaml.Unmarshal([]byte(raw), &doc) != nil {
		return
	}
	if n, ok := doc["name"].(string); ok && strings.TrimSpace(n) != "" {
		r.Name = strings.TrimSpace(n)
	}
	if d, ok := doc["description"].(string); ok {
		r.Description = strings.TrimSpace(d)
	}
}

// parseCriteria reads the `## C1 - title` sections of a skill body; a fenced block never starts one.
func parseCriteria(skill, body string) []Criterion {
	var out []Criterion
	var cur *Criterion
	var text []string
	flush := func() {
		if cur != nil {
			cur.Text = strings.Join(strings.Fields(strings.Join(text, " ")), " ")
			out = append(out, *cur)
		}
		cur, text = nil, nil
	}
	inFence := false
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
		}
		if !inFence {
			if m := criterionHeadRe.FindStringSubmatch(strings.TrimRight(line, " \t\r")); m != nil {
				flush()
				cur = &Criterion{ID: "rubric:" + skill + "/" + m[1], Title: strings.TrimSpace(m[2])}
				continue
			}
			if strings.HasPrefix(line, "## ") {
				flush()
				continue
			}
		}
		if cur != nil {
			text = append(text, line)
		}
	}
	flush()
	return out
}

// Block renders the rubric for a reviewer: the instruction to evaluate every criterion, then the criteria.
func (r Rubric) Block() string {
	var b strings.Builder
	fmt.Fprintf(&b, "[rubric skill=%s sha256=%s tier=%s]\n", r.Name, r.SHA256, r.Tier)
	b.WriteString("Evaluate EVERY criterion below against the files under review and list each one in criteria_results with its id and\n" +
		"pass, fail or not_applicable (a one-phrase note; say why for not_applicable). For each criterion that fails, also report a\n" +
		"finding with code JR-G06, rubric_id set to the criterion id, and a verbatim quote of the file text that fails it (the nearest\n" +
		"anchor, such as the heading or the description, when the failure is an absence).\n")
	for _, c := range r.Criteria {
		fmt.Fprintf(&b, "%s - %s: %s\n", c.ID, c.Title, c.Text)
	}
	return strings.TrimRight(b.String(), "\n")
}

// CriterionIDs returns the ids of all criteria.
func (r Rubric) CriterionIDs() []string {
	ids := make([]string, len(r.Criteria))
	for i, c := range r.Criteria {
		ids[i] = c.ID
	}
	return ids
}
