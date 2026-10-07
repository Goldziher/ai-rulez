package review

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
)

// Dimension calibration statuses.
const (
	CalPass         = "pass"
	CalFail         = "fail"
	CalIllDefined   = "ill-defined"
	CalUncalibrated = "uncalibrated"
)

// Calibration states of a judged run.
const (
	// CalMatched: a passing record for this rubric, prompt, golden set, model and content mode.
	CalMatched = "matched"
	// CalMissing: no record exists.
	CalMissing = "missing"
	// CalStale: a record exists but does not describe the judge that ran.
	CalStale = "stale"
	// CalFailedRecord: the record matches but it did not meet the thresholds.
	CalFailedRecord = "failed"
)

// builtinCalibrationDir is where the record of a built-in rubric lives, under the config directory.
const builtinCalibrationDir = "calibration"

// CalibrationRubric names the rubric a record was measured for.
type CalibrationRubric struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
	Digest  string `json:"digest"`
}

// Curve is one point of the calibration curve: how often a flagged verdict with this vote
// agreement was a true flag.
type CurvePoint struct {
	Agreement float64 `json:"agreement"`
	N         int     `json:"n"`
	Precision float64 `json:"precision"`
}

// DimCalibration is the measured quality of the judge on one dimension.
//
//nolint:tagliatelle // calibration keys are snake_case by project convention
type DimCalibration struct {
	Status string `json:"status"`
	// N is how many golden cases labeled the dimension and were judged.
	N int `json:"n"`
	// Errors counts golden cases the judge could not answer for this dimension.
	Errors      int        `json:"errors,omitempty"`
	Kappa       float64    `json:"kappa"`
	Precision   float64    `json:"precision"`
	PrecisionCI [2]float64 `json:"precision_ci"`
	Recall      float64    `json:"recall"`
	RecallCI    [2]float64 `json:"recall_ci"`
	F1          float64    `json:"f1"`
	// HumanKappa is the labelers' agreement; below the rubric's threshold the dimension is ill-defined.
	HumanKappa  float64 `json:"human_kappa"`
	Consistency float64 `json:"consistency"`
	// FleissKappa is the agreement of the k votes of the judge beyond chance.
	FleissKappa float64 `json:"fleiss_kappa"`
	// UnstableShare is the share of flagged verdicts whose votes disagreed.
	UnstableShare float64            `json:"unstable_share"`
	Metamorphic   map[string]float64 `json:"metamorphic,omitempty"`
	Curve         []CurvePoint       `json:"curve,omitempty"`
	// Misses lists the thresholds the dimension did not meet.
	Misses []string `json:"misses,omitempty"`
}

// CalibrationRecord is calibration.json: a judge measured against a golden set.
//
//nolint:tagliatelle // calibration keys are snake_case by project convention
type CalibrationRecord struct {
	SchemaVersion int               `json:"schema_version"`
	Rubric        CalibrationRubric `json:"rubric"`
	// Model is the model id the provider reported (the resolved id), not the alias requested.
	Model        string `json:"model"`
	PromptDigest string `json:"prompt_digest"`
	GoldenDigest string `json:"golden_digest"`
	// Content is the content mode the judge was calibrated with (descriptions or full).
	Content string `json:"content"`
	Date    string `json:"date"`
	K       int    `json:"k"`
	NItems  int    `json:"n_items"`
	// Dimensions maps a dimension id to its measurements.
	Dimensions map[string]DimCalibration `json:"dimensions"`
	// Cases keeps the aggregated verdict of every golden case, so a later run can print what changed.
	Cases  map[string]map[string]string `json:"cases,omitempty"`
	Status string                       `json:"status"`
}

// CalibrationPath is where the record of rb is kept: calibration.json in the rubric
// directory, or <config dir>/calibration/<id>.builtin.json for a built-in rubric.
func CalibrationPath(configDir string, rb *Rubric) string {
	if rb.Dir != "" {
		return filepath.Join(rb.Dir, CalibrationFile)
	}
	return filepath.Join(configDir, builtinCalibrationDir, rb.ID+".builtin.json")
}

// LoadCalibration reads a record; (nil, nil) when the file does not exist.
func LoadCalibration(path string) (*CalibrationRecord, error) {
	data, _, err := safefs.ReadRegularKeepMode(path)
	if err != nil {
		if isNotExist(err) {
			return nil, nil
		}
		return nil, oops.Wrapf(err, "read calibration record")
	}
	var rec CalibrationRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, oops.Wrapf(err, "parse %s", path)
	}
	if rec.SchemaVersion != SchemaVersion {
		return nil, oops.Errorf("%s has schema_version %d; this build reads %d", path, rec.SchemaVersion, SchemaVersion)
	}
	return &rec, nil
}

// SaveCalibration writes a record (committed to the repository: mode 0644).
func SaveCalibration(path string, rec *CalibrationRecord) error {
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return oops.Wrapf(err, "encode calibration record")
	}
	return oops.Wrapf(safefs.WriteFileAtomicMode(path, append(data, '\n'), 0o644), "write calibration record") //nolint:mnd // a committed file
}

// CalKey is what the judge that runs now looks like; a record must describe exactly this.
type CalKey struct {
	// Model is the resolved model id; "" before the first call (the check is then partial).
	Model        string
	PromptDigest string
	// GoldenDigest is "" when the golden set is not available to recompute (a built-in rubric).
	GoldenDigest string
	Content      string
	K            int
}

// CalStatus is the verdict of matching a record to a run.
//
//nolint:tagliatelle // report keys are snake_case by project convention
type CalStatus struct {
	// State is matched, missing, stale or failed.
	State   string   `json:"status"`
	Reasons []string `json:"reasons,omitempty"`
	// RecordDigest identifies the record file's content.
	RecordDigest string `json:"record_digest,omitempty"`
	Date         string `json:"date,omitempty"`
	AgeDays      int    `json:"age_days,omitempty"`
	Model        string `json:"model,omitempty"`
	// Dimensions lists the dimensions whose calibration passed: the only ones that may gate.
	Dimensions []string `json:"calibrated_dimensions,omitempty"`
}

// MatchCalibration decides whether rec describes the judge in cur. maxAgeDays overrides the
// rubric's calibration.max_age_days when positive.
func MatchCalibration(rb *Rubric, rec *CalibrationRecord, cur CalKey, now time.Time, maxAgeDays int) CalStatus {
	if rec == nil {
		return CalStatus{State: CalMissing, Reasons: []string{"no calibration record: run `ai-rulez review calibrate`"}}
	}
	st := CalStatus{State: CalMatched, Date: rec.Date, Model: rec.Model, RecordDigest: recordDigest(rec)}
	stale := func(format string, args ...any) {
		st.State = CalStale
		st.Reasons = append(st.Reasons, fmt.Sprintf(format, args...))
	}
	if rec.Rubric.ID != rb.ID || rec.Rubric.Digest != rb.CoreDigest {
		stale("the rubric changed since it was calibrated (calibrated for %s@%d, now %s@%d)", rec.Rubric.ID, rec.Rubric.Version, rb.ID, rb.Version)
	}
	if rec.PromptDigest != cur.PromptDigest {
		stale("the prompt changed since it was calibrated")
	}
	if cur.GoldenDigest != "" && rec.GoldenDigest != cur.GoldenDigest {
		stale("the golden set changed since it was calibrated")
	}
	if cur.Model != "" && !SameModel(rec.Model, cur.Model) {
		stale("calibrated for model %s, judging with %s", rec.Model, cur.Model)
	}
	if rec.Content != "" && cur.Content != "" && rec.Content != cur.Content {
		stale("calibrated with --content %s, judging with --content %s", rec.Content, cur.Content)
	}
	if cur.K > 0 && rec.K != cur.K {
		stale("calibrated with k=%d votes, judging with k=%d", rec.K, cur.K)
	}
	limit := maxAgeDays
	if limit <= 0 {
		limit = rb.Calibration.MaxAgeDays
	}
	if limit <= 0 {
		limit = config.DefaultReviewGateMaxAgeDays
	}
	if t, err := time.Parse("2006-01-02", rec.Date); err == nil {
		st.AgeDays = int(now.Sub(t).Hours() / 24)
		if st.AgeDays < -1 {
			stale("the record is dated %s, in the future; a record cannot outlive its own date", rec.Date)
		}
		if st.AgeDays > limit {
			stale("the record is %d days old; the limit is %d", st.AgeDays, limit)
		}
	} else {
		stale("the record has no valid date")
	}
	if st.State == CalMatched && rec.Status != "pass" {
		st.State = CalFailedRecord
		st.Reasons = append(st.Reasons, "the judge did not meet the calibration thresholds")
	}
	for id, d := range rec.Dimensions {
		if d.Status == CalPass {
			st.Dimensions = append(st.Dimensions, id)
		}
	}
	sortStrings(st.Dimensions)
	return st
}

func recordDigest(rec *CalibrationRecord) string {
	data, err := json.Marshal(rec)
	if err != nil {
		return ""
	}
	return digestOf([]fileBytes{{CalibrationFile, data}})
}

// SameModel compares model ids ignoring a provider prefix ("gemini/gemini-2.5-flash" and
// "gemini-2.5-flash" are one model), and the "models/" prefix some providers report.
func SameModel(a, b string) bool { return trimModel(a) == trimModel(b) }

// TrimModel drops the provider prefix of a model id.
func TrimModel(m string) string { return trimModel(m) }

func trimModel(m string) string {
	m = strings.TrimPrefix(m, "models/")
	if i := strings.LastIndex(m, "/"); i >= 0 {
		m = m[i+1:]
	}
	return m
}

var aliasRe = regexp.MustCompile(`(?i)(^|[-_.])(latest|auto|default|stable|current)($|[-_.])`)

// IsFloatingAlias reports whether a model name looks like an alias that the provider may
// repoint at a different model ("...-latest"). Such a model can be used for an advisory run
// and cannot gate.
func IsFloatingAlias(model string) bool { return aliasRe.MatchString(trimModel(model)) }

// GateResult is the outcome of --gate.
//
//nolint:tagliatelle // report keys are snake_case by project convention
type GateResult struct {
	Requested bool   `json:"requested"`
	Level     string `json:"level"`
	// Passed is true when no stable fail verdict at or above Level exists on a calibrated dimension.
	Passed bool `json:"passed"`
	// Refused says why the gate could not be evaluated (exit 1): no matching calibration, an alias model.
	Refused  string        `json:"refused,omitempty"`
	Failures []GateFailure `json:"failures,omitempty"`
}

// GateFailure is one verdict that fails the gate.
type GateFailure struct {
	Item      string  `json:"item"`
	Dimension string  `json:"dimension"`
	Code      string  `json:"code"`
	Agreement float64 `json:"agreement"`
}

var severityRank = map[string]int{"info": 0, "warning": 1, "error": 2}

// EvaluateGate fails when a stable fail verdict exists on a calibrated dimension whose
// severity ceiling is at or above level. calibrated nil means every dimension counts
// (require_calibration = false). Unstable, preempted and errored dimensions never gate.
func EvaluateGate(res *Results, level string, calibrated map[string]bool) GateResult {
	g := GateResult{Requested: true, Level: level, Passed: true}
	floor := severityRank[level]
	for i := range res.Items {
		it := &res.Items[i]
		if it.Semantic == nil {
			continue
		}
		for _, d := range it.Semantic.Dimensions {
			if d.Status != SemJudged || d.Verdict != VerdictFail || d.Capped {
				continue
			}
			if calibrated != nil && !calibrated[d.ID] {
				continue
			}
			if severityRank[d.severity] < floor {
				continue
			}
			if res.baselined[fingerprint(d.Code, it.ID, d.ID, normSpace(firstQuote(d)))] {
				continue
			}
			g.Failures = append(g.Failures, GateFailure{Item: it.ID, Dimension: d.ID, Code: d.Code, Agreement: d.Agreement})
		}
	}
	g.Passed = len(g.Failures) == 0
	return g
}
