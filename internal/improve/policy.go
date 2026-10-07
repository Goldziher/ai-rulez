package improve

import (
	"bytes"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
	"gopkg.in/yaml.v3"
)

// skillFile is the file every skill has; its frontmatter is policed.
const skillFile = "SKILL.md"

// growthFactor bounds how much larger SKILL.md may get.
const growthFactor = 1.25

// Constraints are the rules an optimizer is told and the policy enforces.
type Constraints struct {
	Editable             []string `json:"editable"`
	FrontmatterImmutable []string `json:"frontmatter_immutable"`
	MaxSkillTokens       int      `json:"max_skill_tokens"`
	Forbid               []string `json:"forbid"`
}

// DefaultConstraints returns the policy for a skill whose SKILL.md has origTokens
// tokens. The allow flags widen it.
func DefaultConstraints(origTokens int, allowFrontmatter, allowScripts bool) Constraints {
	return ConstraintsFor(origTokens, growthFactor, allowFrontmatter, allowScripts)
}

// MaxGrowthLimit is the largest max_skill_growth a configuration may ask for.
const MaxGrowthLimit = 2.0

// EffectiveGrowth is the growth factor ConstraintsFor applies for a configured one.
func EffectiveGrowth(growth float64) float64 {
	if growth < 1 || growth > MaxGrowthLimit || math.IsNaN(growth) {
		return growthFactor
	}
	return growth
}

// frontmatterName is the frontmatter key that names a skill.
const frontmatterName = "name"

// ConstraintsFor is DefaultConstraints with an explicit growth factor
// ([improve] max_skill_growth); a factor outside [1, MaxGrowthLimit] falls back to the default.
func ConstraintsFor(origTokens int, growth float64, allowFrontmatter, allowScripts bool) Constraints {
	growth = EffectiveGrowth(growth)
	c := Constraints{
		Editable:             []string{skillFile, "references/**"},
		FrontmatterImmutable: []string{frontmatterName, "allowed-tools", "disable-model-invocation", "model"},
		Forbid:               []string{"scripts/**", "assets/**"},
		MaxSkillTokens:       int(math.Ceil(float64(origTokens) * growth)),
	}
	if allowFrontmatter {
		c.FrontmatterImmutable = []string{frontmatterName}
	}
	if allowScripts {
		c.Editable = append(c.Editable, "scripts/**", "assets/**")
		c.Forbid = nil
	}
	return c
}

// Violation is one broken diff-policy rule (AR9J3).
type Violation struct {
	Code   string `json:"code"`
	Rule   string `json:"rule"`
	Path   string `json:"path,omitempty"`
	Detail string `json:"detail"`
}

// String renders the violation for a terminal. The path and the detail can carry text an
// optimizer chose (a file name, a frontmatter value), so both are sanitized.
func (v Violation) String() string {
	detail := Sanitize(v.Detail, 400)
	if v.Path == "" {
		return fmt.Sprintf("%s %s: %s", v.Code, Sanitize(v.Rule, 80), detail)
	}
	return fmt.Sprintf("%s %s: %s (%s)", v.Code, Sanitize(v.Rule, 80), detail, Sanitize(v.Path, 200))
}

func violation(rule, path, format string, args ...any) Violation {
	return Violation{Code: CodePolicyViolation, Rule: rule, Path: path, Detail: fmt.Sprintf(format, args...)}
}

// PolicyInput is everything CheckDiff needs.
type PolicyInput struct {
	Original, Candidate *Tree
	Constraints         Constraints
	AllowScripts        bool
	Counter             tokens.Counter
	// HeldAssertionValues are assertion strings of held-out cases; one that
	// appears in the candidate but not in the original is reported as a leak.
	HeldAssertionValues []string
}

// CheckDiff applies the diff policy to a candidate. It reads nothing from disk
// and spends nothing.
func CheckDiff(in *PolicyInput) (violations []Violation, warnings []string) {
	orig, cand, c := in.Original, in.Candidate, &in.Constraints
	for _, odd := range cand.Odd {
		violations = append(violations, violation("not-regular", odd, "symlinks, hard links, special files and oversized files are not allowed"))
	}
	for _, p := range cand.Paths() {
		violations = append(violations, checkPath(p, orig, cand, c)...)
	}
	for _, p := range orig.Paths() {
		if _, kept := cand.Files[p]; !kept && !matchAny(c.Editable, p) {
			violations = append(violations, violation("outside-editable", p, "deleted a file the optimizer may not change"))
		}
	}
	violations = append(violations, checkSkillFile(orig, cand, c, in.Counter, in.AllowScripts)...)
	violations = append(violations, newFindings(orig, cand)...)
	warnings = leakWarnings(orig, cand, in.HeldAssertionValues)
	sort.SliceStable(violations, func(i, j int) bool {
		if violations[i].Path != violations[j].Path {
			return violations[i].Path < violations[j].Path
		}
		return violations[i].Rule < violations[j].Rule
	})
	return violations, warnings
}

func checkPath(p string, orig, cand *Tree, c *Constraints) []Violation {
	var out []Violation
	now := cand.Files[p]
	before, existed := orig.Files[p]
	if existed && bytes.Equal(before.Data, now.Data) && before.Exec == now.Exec {
		return nil
	}
	if !matchAny(c.Editable, p) || matchAny(c.Forbid, p) {
		out = append(out, violation("outside-editable", p, "the optimizer may only change %s", strings.Join(c.Editable, ", ")))
	}
	if existed && before.Exec != now.Exec || !existed && now.Exec {
		out = append(out, violation("mode-change", p, "the executable bit changed"))
	}
	return out
}

func checkSkillFile(orig, cand *Tree, c *Constraints, counter tokens.Counter, allowScripts bool) []Violation {
	var out []Violation
	now, ok := cand.Files[skillFile]
	if !ok {
		return []Violation{violation("skill-missing", skillFile, "SKILL.md was removed")}
	}
	before := orig.Files[skillFile]
	oldFM, _, oldOK := splitFrontmatter(before.Data)
	newFM, _, newOK := splitFrontmatter(now.Data)
	switch {
	case !newOK && oldOK:
		out = append(out, violation("frontmatter-malformed", skillFile, "the frontmatter is missing, unclosed or not valid YAML"))
	case newOK:
		for _, key := range c.FrontmatterImmutable {
			_, hadKey := oldFM[key]
			_, hasKey := newFM[key]
			if hadKey != hasKey || !sameValue(oldFM[key], newFM[key]) {
				out = append(out, violation("frontmatter-immutable", skillFile, "frontmatter key %q must not change", key))
			}
		}
	}
	if counter != nil && c.MaxSkillTokens > 0 {
		if n := counter.Count(string(now.Data)); n > c.MaxSkillTokens {
			out = append(out, violation("token-growth", skillFile, "SKILL.md is %d tokens, the limit is %d", n, c.MaxSkillTokens))
		}
	}
	if !allowScripts && scriptRefs(cand) > scriptRefs(orig) {
		out = append(out, violation("script-reference", skillFile, "a reference to scripts/ was added"))
	}
	return out
}

func scriptRefs(t *Tree) int {
	n := 0
	for p, e := range t.Files {
		if p == skillFile || strings.HasPrefix(p, "references/") {
			n += strings.Count(string(e.Data), "scripts/")
		}
	}
	return n
}

func sameValue(a, b any) bool {
	x, _ := yaml.Marshal(a) //nolint:errcheck // scalars and lists always marshal
	y, _ := yaml.Marshal(b) //nolint:errcheck // scalars and lists always marshal
	return bytes.Equal(x, y)
}

// splitFrontmatter parses the YAML block between the first two "---" lines.
func splitFrontmatter(data []byte) (fm map[string]any, body string, ok bool) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	rest, found := strings.CutPrefix(text, "---\n")
	if !found {
		return nil, text, false
	}
	block, body, found := strings.Cut(rest, "\n---")
	if !found {
		return nil, text, false
	}
	if err := yaml.Unmarshal([]byte(block), &fm); err != nil {
		return nil, text, false
	}
	if fm == nil {
		fm = map[string]any{}
	}
	return fm, body, true
}

// Description returns the frontmatter description of a SKILL.md.
func Description(data []byte) string {
	fm, _, ok := splitFrontmatter(data)
	if !ok {
		return ""
	}
	s, isString := fm["description"].(string)
	if !isString {
		return ""
	}
	return strings.TrimSpace(s)
}

// newFindings reports security findings the candidate adds to the files it
// changed. Findings already in the original never block: only new ones do.
func newFindings(orig, cand *Tree) []Violation {
	var out []Violation
	for _, p := range cand.Paths() {
		now := cand.Files[p]
		before, existed := orig.Files[p]
		if existed && bytes.Equal(before.Data, now.Data) {
			continue
		}
		have := map[string]int{}
		if existed {
			for _, f := range lint.ScanText(p, string(before.Data)) {
				have[f.Code+"\x00"+f.Message]++
			}
		}
		for _, f := range lint.ScanText(p, string(now.Data)) {
			key := f.Code + "\x00" + f.Message
			if have[key] > 0 {
				have[key]--
				continue
			}
			out = append(out, violation("new-security-finding", p, "%s %s: %s", f.Code, f.Name, f.Message))
		}
	}
	return out
}

// leakWarnings flags held-out assertion strings the candidate introduced: the
// optimizer cannot have seen them, so a match means a leak or a coincidence a
// reviewer should look at.
func leakWarnings(orig, cand *Tree, values []string) []string {
	var warns []string
	for _, v := range dedupe(values) {
		if len(v) < 4 {
			continue
		}
		for _, p := range cand.Paths() {
			if bytes.Contains(cand.Files[p].Data, []byte(v)) && !bytes.Contains(orig.Files[p].Data, []byte(v)) {
				warns = append(warns, fmt.Sprintf("%s contains a held-out assertion string that the original did not (possible leak)", Sanitize(p, 200)))
				break
			}
		}
	}
	return warns
}

func dedupe(in []string) []string {
	out := slices.Clone(in)
	sort.Strings(out)
	return slices.Compact(out)
}
