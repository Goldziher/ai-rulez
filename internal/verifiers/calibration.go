package verifiers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"time"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
)

// Calibration statuses of a record.
const (
	CalibrationPass = "pass"
	CalibrationFail = "fail"
)

const (
	calibrationSchemaVersion = 1
	// calibrationDir is the directory of the records under verifiers/.
	calibrationDir = "calibration"
	// CalibrationMinPrecision is the share of the model's `fail` verdicts that
	// must be right on the labeled examples before a verifier may gate.
	CalibrationMinPrecision = 0.8
	// CalibrationMinExamples is how many labeled examples a verifier needs, and
	// CalibrationMinPerClass how many of each expected outcome (pass and fail).
	CalibrationMinExamples = 10
	CalibrationMinPerClass = 3
	// wilsonZ is the normal quantile of the 95% interval.
	wilsonZ = 1.96
)

// Calibration is the recorded measurement of an llm verifier against its
// labeled examples: how often its `fail` verdict was right (precision) and how
// many real failures it found (recall). It is committed beside the declaration
// (.ai-rulez/verifiers/calibration/<id>.json), and a verifier gates only while
// the record describes the verifier that runs now. The fields follow the review
// calibration record where they mean the same thing.
//
//nolint:tagliatelle // calibration keys are snake_case by project convention
type Calibration struct {
	SchemaVersion int    `json:"schema_version"`
	Verifier      string `json:"verifier"`
	// SpecDigest covers the llm predicates (checklist, model, max_diff_bytes) and
	// the prompt version; ExamplesDigest covers the labeled examples.
	SpecDigest     string `json:"spec_digest"`
	ExamplesDigest string `json:"examples_digest"`
	PromptVersion  string `json:"prompt_version"`
	// Model is the model the examples were judged with.
	Model string `json:"model"`
	Date  string `json:"date"`
	// NItems counts the examples run; Positives those expected to fail.
	NItems    int `json:"n_items"`
	Positives int `json:"positives"`
	// Errors counts examples that could not be evaluated (model refusal, an
	// unusable reply, a budget stop); any makes the record fail.
	Errors      int        `json:"errors,omitempty"`
	TP          int        `json:"tp"`
	FP          int        `json:"fp"`
	FN          int        `json:"fn"`
	TN          int        `json:"tn"`
	Precision   float64    `json:"precision"`
	PrecisionCI [2]float64 `json:"precision_ci"`
	Recall      float64    `json:"recall"`
	RecallCI    [2]float64 `json:"recall_ci"`
	F1          float64    `json:"f1"`
	Status      string     `json:"status"`
	// Misses lists the thresholds the verifier did not meet.
	Misses []string `json:"misses,omitempty"`
	// Cases keeps the outcome of every example, so a later run can say what changed.
	Cases map[string]string `json:"cases,omitempty"`
}

// CalibrationReport is the outcome of Calibrate.
type CalibrationReport struct {
	Records []Calibration `json:"records"`
	LLM     *LLMUsage     `json:"llm,omitempty"`
	// Estimates is set instead of Records when only an estimate was asked for.
	Estimates []string `json:"estimates,omitempty"`
}

// CalibrateOptions selects the verifiers to calibrate.
type CalibrateOptions struct {
	// Names restricts the run to these verifiers; empty calibrates every llm verifier.
	Names []string
	// LLM is the model access; a Client is required unless Estimate is set.
	LLM LLMOptions
	// Now is the date recorded; the caller supplies it.
	Now time.Time
}

// Calibrate runs the labeled examples of the llm verifiers through the model
// and measures how reliable their `fail` verdicts are. It writes nothing; see
// SaveCalibration.
func Calibrate(ctx context.Context, cfg *config.Config, opts CalibrateOptions) (*CalibrationReport, error) {
	if opts.LLM.Client == nil && !opts.LLM.Estimate {
		reason := opts.LLM.Disabled
		if reason == "" {
			reason = llmOffHint
		}
		return nil, oops.Errorf("calibration needs a model: %s", reason)
	}
	specs, problems := LoadSpecs(cfg)
	if len(problems) > 0 {
		return nil, oops.Errorf("a verifier declaration is invalid: %s", problems[0].Message)
	}
	picked, err := pickLLMSpecs(specs, opts.Names)
	if err != nil {
		return nil, err
	}
	shared := &llmRun{opts: opts.LLM}
	shared.usage.MaxCostUSD = opts.LLM.MaxCostUSD
	rep := &CalibrationReport{}
	for _, sp := range picked {
		if opts.LLM.Estimate {
			rep.Estimates = append(rep.Estimates, estimateExamples(ctx, cfg, sp, opts.LLM, shared)...)
			continue
		}
		rep.Records = append(rep.Records, calibrateSpec(ctx, cfg, sp, opts, shared))
	}
	if shared.usage.Calls > 0 {
		usage := shared.usage
		rep.LLM = &usage
	}
	return rep, nil
}

func pickLLMSpecs(specs []Spec, names []string) ([]*Spec, error) {
	byID := map[string]*Spec{}
	for i := range specs {
		byID[specs[i].ID] = &specs[i]
	}
	var out []*Spec
	if len(names) == 0 {
		for i := range specs {
			if usesLLM(specs[i].Require) {
				out = append(out, &specs[i])
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
		return out, nil
	}
	for _, n := range names {
		sp, ok := byID[n]
		if !ok {
			return nil, oops.Hint("Run `ai-rulez verifiers list` to see the verifiers.").Errorf("unknown verifier %q", n)
		}
		if !usesLLM(sp.Require) {
			return nil, oops.Errorf("verifier %q has no llm predicate: only a model's verdict needs calibrating (`ai-rulez verifiers test` runs the examples of the others)", n)
		}
		out = append(out, sp)
	}
	return out, nil
}

func estimateExamples(ctx context.Context, cfg *config.Config, sp *Spec, o LLMOptions, shared *llmRun) []string {
	var out []string
	for _, ex := range sp.Examples {
		res := runExample(ctx, cfg, sp, ex, Options{LLM: &o, sharedLLM: shared})
		out = append(out, fmt.Sprintf("%s: %s: %s", sp.ID, ex.Name, res.Message))
	}
	return out
}

// calibrateSpec measures one verifier. An example counts as flagged when the
// verifier fails on it; it is a true flag when the example is labeled `fail`.
func calibrateSpec(ctx context.Context, cfg *config.Config, sp *Spec, opts CalibrateOptions, shared *llmRun) Calibration {
	rec := Calibration{
		SchemaVersion: calibrationSchemaVersion, Verifier: sp.ID, SpecDigest: specDigest(sp), ExamplesDigest: examplesDigest(sp.Examples),
		PromptVersion: LLMPromptVersion, Model: effectiveModel(sp, opts.LLM.Model), Date: opts.Now.UTC().Format("2006-01-02"),
		Cases: map[string]string{},
	}
	expectedPass := 0
	for _, ex := range sp.Examples {
		res := runExample(ctx, cfg, sp, ex, Options{LLM: &opts.LLM, sharedLLM: shared})
		rec.NItems++
		rec.Cases[ex.Name] = string(res.Got)
		positive := ex.Expect == string(StatusFail)
		if positive {
			rec.Positives++
		} else {
			expectedPass++
		}
		switch {
		case res.Got != StatusFail && res.Got != StatusPass && res.Got != StatusNotApplicable:
			rec.Errors++
		case res.Got == StatusFail && positive:
			rec.TP++
		case res.Got == StatusFail:
			rec.FP++
		case positive:
			rec.FN++
		default:
			rec.TN++
		}
	}
	rec.Precision, rec.PrecisionCI = ratio(rec.TP, rec.TP+rec.FP)
	rec.Recall, rec.RecallCI = ratio(rec.TP, rec.TP+rec.FN)
	if rec.Precision+rec.Recall > 0 {
		rec.F1 = 2 * rec.Precision * rec.Recall / (rec.Precision + rec.Recall)
	}
	rec.Misses = calibrationMisses(&rec, expectedPass)
	rec.Status = CalibrationPass
	if len(rec.Misses) > 0 {
		rec.Status = CalibrationFail
	}
	return rec
}

func calibrationMisses(rec *Calibration, expectedPass int) []string {
	var misses []string
	if rec.NItems < CalibrationMinExamples || rec.Positives < CalibrationMinPerClass || expectedPass < CalibrationMinPerClass {
		misses = append(misses, fmt.Sprintf("needs at least %d labeled examples with at least %d expected to fail and %d to pass (has %d: %d fail, %d pass)",
			CalibrationMinExamples, CalibrationMinPerClass, CalibrationMinPerClass, rec.NItems, rec.Positives, expectedPass))
	}
	if rec.Errors > 0 {
		misses = append(misses, fmt.Sprintf("%d example(s) could not be evaluated", rec.Errors))
	}
	if rec.Precision < CalibrationMinPrecision {
		misses = append(misses, fmt.Sprintf("precision %.2f is below %.2f (%d of %d flagged examples were real failures)", rec.Precision, CalibrationMinPrecision, rec.TP, rec.TP+rec.FP))
	}
	return misses
}

// ratio is k/n with its 95% Wilson interval; no observations give 0 and [0,1].
func ratio(k, n int) (rate float64, interval [2]float64) {
	if n == 0 {
		return 0, [2]float64{0, 1}
	}
	p := float64(k) / float64(n)
	z2 := wilsonZ * wilsonZ
	denom := 1 + z2/float64(n)
	center := (p + z2/(2*float64(n))) / denom
	margin := wilsonZ * math.Sqrt(p*(1-p)/float64(n)+z2/(4*float64(n)*float64(n))) / denom
	return p, [2]float64{math.Max(0, center-margin), math.Min(1, center+margin)}
}

// effectiveModel is the model the verifier's calls use: its own, else the run's.
func effectiveModel(sp *Spec, def string) string {
	for _, p := range llmPreds(sp.Require) {
		if p.Model != "" {
			return p.Model
		}
	}
	return def
}

// llmPreds lists the llm predicates of a predicate tree in order.
func llmPreds(r *Require) []*LLMPred {
	if r == nil {
		return nil
	}
	var out []*LLMPred
	if r.LLM != nil {
		out = append(out, r.LLM)
	}
	for _, kids := range [][]Require{r.All, r.Any} {
		for i := range kids {
			out = append(out, llmPreds(&kids[i])...)
		}
	}
	return append(out, llmPreds(r.Not)...)
}

// specDigest covers everything that decides the verifier's verdict around its
// llm predicates: the whole predicate tree (a calibrated predicate wrapped in
// `not`, or given `any`/`all` siblings, is a different check), the file scope
// and the prompt version. A record whose digest differs stops gating.
func specDigest(sp *Spec) string {
	return digestOf(struct {
		Schema      string   `json:"schema"`
		Require     *Require `json:"require"`
		WhenChanged []string `json:"when_changed"`
		Exclude     []string `json:"exclude"`
		Prompt      string   `json:"prompt"`
	}{"verifier-spec/2", sp.Require, sp.WhenChanged, sp.Exclude, LLMPromptVersion})
}

func examplesDigest(exs []Example) string {
	sorted := append([]Example(nil), exs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	return digestOf(sorted)
}

func digestOf(v any) string {
	data, err := json.Marshal(v) // map keys are sorted, so the encoding is stable
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// calibrationPath is the record of a verifier; an id with a path separator has none.
func calibrationPath(cfg *config.Config, id string) (string, error) {
	if id == "" || id != filepath.Base(id) {
		return "", oops.Errorf("verifier id %q cannot name a calibration record", id)
	}
	return filepath.Join(cfg.ConfigDir, VerifiersDirName, calibrationDir, id+".json"), nil
}

// SaveCalibration writes a record under .ai-rulez/verifiers/calibration/ (a
// committed file, mode 0644).
func SaveCalibration(cfg *config.Config, rec *Calibration) error {
	path, err := calibrationPath(cfg, rec.Verifier)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return oops.Wrapf(err, "encode calibration record")
	}
	return oops.Wrapf(safefs.WriteFileAtomicMode(path, append(data, '\n'), 0o644), "write calibration record") //nolint:mnd // a committed file
}

// LoadCalibration reads the record of a verifier; (nil, nil) when there is none.
func LoadCalibration(cfg *config.Config, id string) (*Calibration, error) {
	path, err := calibrationPath(cfg, id)
	if err != nil {
		return nil, err
	}
	data, _, err := safefs.ReadRegularKeepMode(path)
	if err != nil {
		if isMissing(err) {
			return nil, nil
		}
		return nil, oops.Wrapf(err, "read calibration record")
	}
	var rec Calibration
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, oops.Wrapf(err, "parse %s", path)
	}
	if rec.SchemaVersion != calibrationSchemaVersion {
		return nil, oops.Errorf("%s has schema_version %d; this build reads %d", path, rec.SchemaVersion, calibrationSchemaVersion)
	}
	return &rec, nil
}

// calibrationVerdict says whether the record lets sp gate when run with model,
// and why not otherwise.
func calibrationVerdict(rec *Calibration, sp *Spec, model string) (ok bool, why string) {
	switch {
	case rec == nil:
		return false, "not calibrated: run `ai-rulez verifiers calibrate " + sp.ID + "` and commit the record"
	case rec.Status != CalibrationPass:
		return false, "its calibration did not meet the bar: " + joinMisses(rec.Misses)
	case rec.SpecDigest != specDigest(sp):
		return false, "the checklist or llm settings changed since it was calibrated (" + rec.Date + ")"
	case rec.ExamplesDigest != examplesDigest(sp.Examples):
		return false, "the examples changed since it was calibrated (" + rec.Date + ")"
	case rec.PromptVersion != LLMPromptVersion:
		return false, "the verifier prompt changed since it was calibrated (" + rec.Date + ")"
	case rec.Model != model:
		return false, fmt.Sprintf("calibrated for model %s, running with %s", rec.Model, nonEmptyOr(model, "the default model"))
	}
	return true, ""
}

func joinMisses(m []string) string {
	if len(m) == 0 {
		return "unknown reason"
	}
	out := m[0]
	for _, s := range m[1:] {
		out += "; " + s
	}
	return out
}

// gateNote is the note of a verifier that gates on its calibration.
func gateNote(rec *Calibration) string {
	return fmt.Sprintf("gated: calibrated on %d examples (%s), precision %.2f (95%% interval %.2f-%.2f), recall %.2f, model %s",
		rec.NItems, rec.Date, rec.Precision, rec.PrecisionCI[0], rec.PrecisionCI[1], rec.Recall, rec.Model)
}
