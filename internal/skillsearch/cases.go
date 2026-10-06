package skillsearch

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
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
)

// Case is one labeled query: the skills that are relevant to it. A case with
// no expected skills is negative: nothing is relevant.
type Case struct {
	ID     string   `yaml:"id"`
	Query  string   `yaml:"query"`
	Expect []string `yaml:"expect"`
	Tags   []string `yaml:"tags"`
}

// CaseFile is a `search --eval` cases file (`version: 1`).
type CaseFile struct {
	Version int    `yaml:"version"`
	K       int    `yaml:"k"`
	Cases   []Case `yaml:"cases"`
}

// LoadCases reads and validates a cases file. Every problem is reported at once
// as one AR9D2 error. Unknown fields (including `role` and graded `expect`
// entries, which later phases add) are errors, so a file never silently means
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
		c := &f.Cases[i]
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
		for _, id := range c.Expect {
			if dup[id] {
				problems = append(problems, fmt.Sprintf("%s: %q is listed twice in expect", where, id))
			}
			dup[id] = true
		}
	}
	return problems
}

// CheckSkills reports, as one AR9D2 error, every expected skill id that is not
// in the catalog the cases run against.
func (f *CaseFile) CheckSkills(known []string) error {
	have := make(map[string]bool, len(known))
	for _, k := range known {
		have[k] = true
	}
	var problems []string
	for i := range f.Cases {
		for _, id := range f.Cases[i].Expect {
			if !have[id] {
				problems = append(problems, fmt.Sprintf("case %q expects unknown skill %q", f.Cases[i].ID, id))
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
