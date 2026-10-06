package review

import (
	"encoding/json"
	"sort"
	"strconv"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
)

// BaselineSchema identifies a baseline file.
const BaselineSchema = "review-baseline/1"

// Baseline is the set of findings a team has accepted: a later run hides them (they never
// gate) and reports only what is new. It holds fingerprints, which survive line moves.
//
//nolint:tagliatelle // baseline keys are snake_case by project convention
type Baseline struct {
	Schema       string   `json:"schema"`
	Rubric       string   `json:"rubric,omitempty"`
	Fingerprints []string `json:"fingerprints"`
}

// NewBaseline records the dimension findings of a report. Run notes (AR9G0, AR9G9) are not
// findings about content and are left out.
func NewBaseline(rep *Report) *Baseline {
	b := &Baseline{Schema: BaselineSchema, Rubric: rep.Rubric.ID + "@" + strconv.Itoa(rep.Rubric.Version), Fingerprints: []string{}}
	seen := map[string]bool{}
	for _, f := range rep.Findings {
		if isNote(f.Code) || seen[f.Fingerprint] {
			continue
		}
		seen[f.Fingerprint] = true
		b.Fingerprints = append(b.Fingerprints, f.Fingerprint)
	}
	sort.Strings(b.Fingerprints)
	return b
}

func isNote(code string) bool {
	return code == lint.CodeReviewRunNote || code == lint.CodeReviewCalibrationStale
}

// LoadBaseline reads a baseline file or a previous `review --format json` report (whose
// findings carry the same fingerprints).
func LoadBaseline(path string) (*Baseline, error) {
	data, _, err := safefs.ReadRegularKeepMode(path)
	if err != nil {
		return nil, oops.Wrapf(err, "read baseline %s", path)
	}
	var probe struct {
		Schema   string    `json:"schema"`
		Findings []Finding `json:"findings"`
		Prints   []string  `json:"fingerprints"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, oops.Wrapf(err, "parse baseline %s", path)
	}
	switch probe.Schema {
	case BaselineSchema:
		return &Baseline{Schema: BaselineSchema, Fingerprints: probe.Prints}, nil
	case ReportSchema:
		b := &Baseline{Schema: BaselineSchema}
		for _, f := range probe.Findings {
			if !isNote(f.Code) && f.Fingerprint != "" {
				b.Fingerprints = append(b.Fingerprints, f.Fingerprint)
			}
		}
		return b, nil
	}
	return nil, oops.Errorf("%s is not a review baseline or a review report (schema %q)", path, probe.Schema)
}

// Set returns the fingerprints as a lookup set.
func (b *Baseline) Set() map[string]bool {
	set := make(map[string]bool, len(b.Fingerprints))
	for _, fp := range b.Fingerprints {
		set[fp] = true
	}
	return set
}

// WriteBaseline writes the baseline of rep to path.
func WriteBaseline(path string, rep *Report) error {
	data, err := json.MarshalIndent(NewBaseline(rep), "", "  ")
	if err != nil {
		return oops.Wrapf(err, "encode baseline")
	}
	return oops.Wrapf(safefs.WriteFileAtomicMode(path, append(data, '\n'), 0o644), "write baseline %s", path) //nolint:mnd // a committed file
}

// SetBaseline hides the findings of set from Findings (they are marked baselined) and from
// the gate. nil clears it.
func (r *Results) SetBaseline(set map[string]bool) { r.baselined = set }

// BaselineCounts returns how many dimension findings are baselined and how many are new.
func (r *Results) BaselineCounts(rb *Rubric) (baselined, fresh int) {
	for _, f := range r.Findings(rb) {
		if isNote(f.Code) || f.Origin == "" {
			continue
		}
		if f.Baselined {
			baselined++
		} else {
			fresh++
		}
	}
	return baselined, fresh
}
