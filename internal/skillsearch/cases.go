package skillsearch

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/samber/oops"
	"gopkg.in/yaml.v3"
)

// Rule codes of the search block (AR9D). AR9D2 and AR9D4 are emitted by
// `search --eval`; they are not yet findings of `validate`.
const (
	CodeCasesInvalid    = "AR9D2" // search-cases-invalid
	CodeEvalRegression  = "AR9D4" // search-eval-regression
	casesSchemaVersion  = 1
	defaultK            = 5
	maxK                = 100
	maxCaseFileBytes    = 4 << 20
	maxQueryBytes       = 4096
	maxCasesPerFile     = 10000
	maxExpectedPerQuery = 50
	maxGrade            = 9
)

// Relevant is one relevant skill of a case, with its grade. A bare skill name
// has grade 1; `{id, grade}` sets one (higher is more relevant) and makes the
// case graded, which enables nDCG.
type Relevant struct {
	ID     string
	Grade  int
	Graded bool
}

// UnmarshalYAML reads `skill-name` or `{id: skill-name, grade: 2}`; any other
// key is an error.
func (r *Relevant) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		r.ID, r.Grade, r.Graded = n.Value, 1, false
		return nil
	case yaml.MappingNode:
		r.Grade, r.Graded = 1, false
		for i := 0; i+1 < len(n.Content); i += 2 {
			key, val := n.Content[i].Value, n.Content[i+1]
			switch key {
			case "id":
				r.ID = val.Value
			case "grade":
				g, err := strconv.Atoi(val.Value)
				if err != nil {
					return fmt.Errorf("line %d: grade %q is not an integer", val.Line, val.Value)
				}
				r.Grade, r.Graded = g, true
			default:
				return fmt.Errorf("line %d: field %q not found in an expect entry (want id and grade)", n.Content[i].Line, key)
			}
		}
		return nil
	}
	return fmt.Errorf("line %d: an expect entry is a skill name or {id, grade}", n.Line)
}

// Case is one labeled query: the skills that are relevant to it. A case with
// no expected skills is negative: nothing is relevant. Avoid lists skills that
// must not rank first (a near-miss of that skill); Role scopes the ranking to the
// skills of that role.
type Case struct {
	ID     string     `yaml:"id"`
	Query  string     `yaml:"query"`
	Expect []Relevant `yaml:"expect"`
	Avoid  []string   `yaml:"avoid"`
	Role   string     `yaml:"role"`
	Tags   []string   `yaml:"tags"`
}

// ExpectIDs returns the ids of the relevant skills.
func (c Case) ExpectIDs() []string {
	out := make([]string, len(c.Expect))
	for i, r := range c.Expect {
		out[i] = r.ID
	}
	return out
}

// CaseFile is a `search --eval` cases file (`version: 1`).
type CaseFile struct {
	Version int    `yaml:"version"`
	K       int    `yaml:"k"`
	Cases   []Case `yaml:"cases"`
}

// LoadCases reads and validates a cases file. Every problem is reported at once
// as one AR9D2 error. Unknown fields are errors, so a file never silently means
// less than it says.
func LoadCases(path string) (*CaseFile, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, oops.Wrapf(err, "read cases file")
	}
	if !info.Mode().IsRegular() {
		return nil, oops.Errorf("cases file %s is not a regular file", path)
	}
	if info.Size() > maxCaseFileBytes {
		return nil, oops.Errorf("cases file %s is larger than %d bytes", path, maxCaseFileBytes)
	}
	raw, err := os.ReadFile(path) //nolint:gosec // an explicit user-supplied path
	if err != nil {
		return nil, oops.Wrapf(err, "read cases file")
	}
	return ParseCases(raw)
}

// ParseCases is LoadCases over bytes.
func ParseCases(raw []byte) (*CaseFile, error) {
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	var f CaseFile
	if err := dec.Decode(&f); err != nil && !errors.Is(err, io.EOF) {
		return nil, oops.Code(CodeCasesInvalid).Errorf("%s: invalid cases file: %v", CodeCasesInvalid, err)
	}
	if problems := f.validate(); len(problems) > 0 {
		return nil, oops.Code(CodeCasesInvalid).Errorf("%s: invalid cases file: %s", CodeCasesInvalid, strings.Join(problems, "; "))
	}
	return &f, nil
}

func (f *CaseFile) validate() []string {
	var problems []string
	if f.Version != casesSchemaVersion {
		problems = append(problems, fmt.Sprintf("version must be %d (got %d)", casesSchemaVersion, f.Version))
	}
	if f.K < 0 || f.K > maxK {
		problems = append(problems, fmt.Sprintf("k must be between 1 and %d", maxK))
	}
	if len(f.Cases) == 0 {
		problems = append(problems, "cases is empty")
	}
	if len(f.Cases) > maxCasesPerFile {
		problems = append(problems, fmt.Sprintf("more than %d cases", maxCasesPerFile))
	}
	seen := map[string]bool{}
	for i := range f.Cases {
		problems = append(problems, validateCase(&f.Cases[i], i, seen)...)
	}
	return problems
}

// validateCase reports the problems of case number i (zero-based); seen holds the ids met so far.
func validateCase(c *Case, i int, seen map[string]bool) []string {
	var problems []string
	where := fmt.Sprintf("case %d", i+1)
	if c.ID != "" {
		where = fmt.Sprintf("case %q", c.ID)
	}
	switch {
	case strings.TrimSpace(c.ID) == "":
		problems = append(problems, where+": id is required")
	case seen[c.ID]:
		problems = append(problems, where+": duplicate id")
	}
	seen[c.ID] = true
	if strings.TrimSpace(c.Query) == "" {
		problems = append(problems, where+": query is required")
	}
	if len(c.Query) > maxQueryBytes {
		problems = append(problems, fmt.Sprintf("%s: query is longer than %d bytes", where, maxQueryBytes))
	}
	if len(c.Expect) > maxExpectedPerQuery {
		problems = append(problems, fmt.Sprintf("%s: more than %d expected skills", where, maxExpectedPerQuery))
	}
	dup := map[string]bool{}
	for _, r := range c.Expect {
		switch {
		case strings.TrimSpace(r.ID) == "":
			problems = append(problems, where+": an expect entry has no id")
		case dup[r.ID]:
			problems = append(problems, fmt.Sprintf("%s: %q is listed twice in expect", where, r.ID))
		}
		dup[r.ID] = true
		if r.Graded && (r.Grade < 1 || r.Grade > maxGrade) {
			problems = append(problems, fmt.Sprintf("%s: grade of %q must be between 1 and %d", where, r.ID, maxGrade))
		}
	}
	for _, id := range c.Avoid {
		if dup[id] {
			problems = append(problems, fmt.Sprintf("%s: %q is both expected and avoided", where, id))
		}
	}
	return problems
}

// CheckSkills reports, as one AR9D2 error, every expected or avoided skill id
// that is not in the catalog the cases run against.
func (f *CaseFile) CheckSkills(known []string) error {
	have := make(map[string]bool, len(known))
	for _, k := range known {
		have[k] = true
	}
	var problems []string
	for i := range f.Cases {
		for _, id := range f.Cases[i].ExpectIDs() {
			if !have[id] {
				problems = append(problems, fmt.Sprintf("case %q expects unknown skill %q", f.Cases[i].ID, id))
			}
		}
		for _, id := range f.Cases[i].Avoid {
			if !have[id] {
				problems = append(problems, fmt.Sprintf("case %q avoids unknown skill %q", f.Cases[i].ID, id))
			}
		}
	}
	if len(problems) == 0 {
		return nil
	}
	sort.Strings(problems)
	return oops.Code(CodeCasesInvalid).Errorf("%s: invalid cases file: %s", CodeCasesInvalid, strings.Join(problems, "; "))
}

// CheckK rejects a --k override outside 0..100 (0 means "use the file's k"), so
// an oversized value is an error rather than silently clamped.
func CheckK(override int) error {
	if override < 0 || override > maxK {
		return oops.Errorf("--k must be between 1 and %d (or 0 for the file's k), got %d", maxK, override)
	}
	return nil
}

// EffectiveK is the cut-off recall and hit are measured at: the override when
// positive, then the file's k, then 5.
func (f *CaseFile) EffectiveK(override int) int {
	switch {
	case override > 0:
		return min(override, maxK)
	case f.K > 0:
		return f.K
	default:
		return defaultK
	}
}
