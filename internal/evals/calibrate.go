package evals

import (
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Kinds of estimate record a calibration reads.
const (
	// KindCases is a full case run, KindActivation a native activation run.
	KindCases      = "cases"
	KindActivation = "activation"
)

// DefaultMinSamples is how many runs a group needs before its proposal is not
// marked low-confidence.
const DefaultMinSamples = 3

// CalibrateOptions configures Calibrate.
type CalibrateOptions struct {
	// Harness and Model, when set, restrict the calibration to those runs.
	Harness, Model string
	// MinSamples marks a group with fewer runs as low-confidence. Default DefaultMinSamples.
	MinSamples int
}

// Calibration is the result of fitting the estimate's assumptions to recorded runs.
type Calibration struct {
	// Groups hold one proposal per harness, model and kind of run: estimate history
	// from different models is never mixed.
	Groups []CalibrationGroup `json:"groups"`
	// Skipped counts records left out, by reason.
	Skipped CalibrationSkips `json:"skipped"`
}

// CalibrationSkips counts the records a calibration could not use.
type CalibrationSkips struct {
	// NoUsage: the runner reported no token split, so there is nothing to fit.
	NoUsage int `json:"no_usage"`
	// Unverified: the record is not signed with this user's key.
	Unverified int `json:"unverified"`
	// Filtered: another harness or model than the one asked for.
	Filtered int `json:"filtered"`
}

// CalibrationGroup is the proposal for one harness, model and kind of run.
type CalibrationGroup struct {
	Harness string `json:"harness"`
	Model   string `json:"model"`
	Kind    string `json:"kind"`
	// Samples is how many recorded runs the proposal is fitted to (one per skill).
	Samples       int  `json:"samples"`
	LowConfidence bool `json:"low_confidence"`
	// Current is what the estimates in the records assumed (the median), Proposed
	// what the recorded runs measured. Zero means "no data for it".
	Current  EstimateParams `json:"current"`
	Proposed EstimateParams `json:"proposed"`
	// TokenError is median(actual/expected - 1) over total tokens, before and after
	// the proposed assumptions; CostErrorMedian and CostErrorP90 are the recorded
	// cost errors (absolute), which the proposal is meant to shrink.
	TokenErrorBefore float64 `json:"token_error_before"`
	TokenErrorAfter  float64 `json:"token_error_after"`
	CostErrorMedian  float64 `json:"cost_error_median"`
	CostErrorP90     float64 `json:"cost_error_p90"`
	// ListPriceIn is the price table's USD per million input tokens for the model and
	// EffectivePriceIn what the runs were really billed per input token (output at
	// the list price): below the list price when the harness caches its prompt. Zero
	// when the model has no listed price or no run reported a cost.
	ListPriceIn      float64 `json:"list_price_in_per_mtok,omitempty"`
	EffectivePriceIn float64 `json:"effective_price_in_per_mtok,omitempty"`
}

// sample is one recorded run reduced to what the fit needs.
type sample struct {
	kind, harness, model string
	rec                  *EstimateRecord
}

// Calibrate fits the overhead and output assumptions to the estimate records in the
// store: for each group, the overhead is moved by the median gap between the input
// tokens the runner reported and the ones the estimate expected, per agent run, and
// the output assumption likewise. It is deterministic, offline and reads only
// records signed with this user's key. The tool-loop factor is left alone: one
// record cannot separate it from the overhead.
func Calibrate(store *Store, opts *CalibrateOptions) *Calibration {
	minSamples := opts.MinSamples
	if minSamples <= 0 {
		minSamples = DefaultMinSamples
	}
	cal := &Calibration{Groups: []CalibrationGroup{}}
	groups := map[[3]string][]sample{}
	for i := range store.Skills {
		rec := &store.Skills[i]
		var cands []sample
		if rec.Estimate != nil {
			cands = append(cands, sample{KindCases, rec.Harness, rec.Model, rec.Estimate})
		}
		if rec.Activation != nil && rec.Activation.Estimate != nil {
			cands = append(cands, sample{KindActivation, rec.Activation.Harness, rec.Activation.Model, rec.Activation.Estimate})
		}
		for _, s := range cands {
			switch {
			case !rec.Verified():
				cal.Skipped.Unverified++
			case (opts.Harness != "" && s.harness != opts.Harness) || (opts.Model != "" && s.model != opts.Model):
				cal.Skipped.Filtered++
			case s.rec.AgentRuns < 1 || (s.rec.ActualInputTokens == 0 && s.rec.ActualOutputTokens == 0):
				cal.Skipped.NoUsage++
			default:
				key := [3]string{s.kind, s.harness, s.model}
				groups[key] = append(groups[key], s)
			}
		}
	}
	keys := make([][3]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(a, b int) bool {
		if keys[a][0] != keys[b][0] {
			return keys[a][0] < keys[b][0]
		}
		if keys[a][1] != keys[b][1] {
			return keys[a][1] < keys[b][1]
		}
		return keys[a][2] < keys[b][2]
	})
	for _, k := range keys {
		cal.Groups = append(cal.Groups, fitGroup(k[0], k[1], k[2], groups[k], minSamples))
	}
	return cal
}

// fitGroup proposes assumptions for one group.
func fitGroup(kind, harness, model string, samples []sample, minSamples int) CalibrationGroup {
	g := CalibrationGroup{Harness: harness, Model: model, Kind: kind, Samples: len(samples), LowConfidence: len(samples) < minSamples}
	var overheads, outputs, curOverheads, curOutputs, errsAbs []float64
	for _, s := range samples {
		rec := s.rec
		p := recordParams(rec)
		runs := float64(rec.AgentRuns)
		curOverheads = append(curOverheads, float64(p.OverheadTokens))
		overheads = append(overheads, float64(p.OverheadTokens)+float64(rec.ActualInputTokens-rec.ExpectedInputTokens)/runs)
		cur := p.AssumedOutputTokens
		if kind == KindActivation {
			cur = p.ActivationOutputTokens
		}
		curOutputs = append(curOutputs, float64(cur))
		outputs = append(outputs, float64(cur)+float64(rec.ActualOutputTokens-rec.ExpectedOutputTokens)/runs)
		if rec.Error != nil {
			errsAbs = append(errsAbs, math.Abs(*rec.Error))
		}
	}
	g.Current.OverheadTokens = int(math.Round(median(curOverheads)))
	g.Proposed.OverheadTokens = max(0, int(math.Round(median(overheads))))
	out := max(0, int(math.Round(median(outputs))))
	curOut := int(math.Round(median(curOutputs)))
	if kind == KindActivation {
		g.Current.ActivationOutputTokens, g.Proposed.ActivationOutputTokens = curOut, out
	} else {
		g.Current.AssumedOutputTokens, g.Proposed.AssumedOutputTokens = curOut, out
	}
	var before, after []float64
	for _, s := range samples {
		rec := s.rec
		actual := float64(rec.ActualInputTokens + rec.ActualOutputTokens)
		p := recordParams(rec)
		expected := float64(rec.ExpectedInputTokens + rec.ExpectedOutputTokens)
		if expected <= 0 || actual <= 0 {
			continue
		}
		before = append(before, actual/expected-1)
		outCur := p.AssumedOutputTokens
		if kind == KindActivation {
			outCur = p.ActivationOutputTokens
		}
		runs := float64(rec.AgentRuns)
		reexpected := expected + runs*float64(g.Proposed.OverheadTokens-p.OverheadTokens) + runs*float64(out-outCur)
		if reexpected > 0 {
			after = append(after, actual/reexpected-1)
		}
	}
	g.TokenErrorBefore, g.TokenErrorAfter = round(median(before)), round(median(after))
	g.ListPriceIn, g.EffectivePriceIn = effectiveInputPrice(model, samples)
	g.CostErrorMedian, g.CostErrorP90 = round(median(errsAbs)), round(quantile(errsAbs, 0.9))
	return g
}

// effectiveInputPrice is the median USD per million input tokens the runs were
// billed, with output tokens at the model's list price; zero when the model has no
// listed price or no sample reported a cost.
func effectiveInputPrice(model string, samples []sample) (list, effective float64) {
	price, known := PriceFor(model)
	if !known {
		return 0, 0
	}
	var per []float64
	for _, s := range samples {
		r := s.rec
		if r.ActualUSD <= 0 || r.ActualInputTokens <= 0 {
			continue
		}
		in := (r.ActualUSD*1e6 - float64(r.ActualOutputTokens)*price.OutPerMTok) / float64(r.ActualInputTokens)
		if in > 0 {
			per = append(per, in)
		}
	}
	if len(per) == 0 {
		return price.InPerMTok, 0
	}
	return price.InPerMTok, math.Round(median(per)*1e4) / 1e4
}

// recordParams are the assumptions a record's estimate was made under.
func recordParams(rec *EstimateRecord) EstimateParams {
	if rec.Params == nil {
		return DefaultEstimateParams()
	}
	return rec.Params.withDefaults()
}

func median(v []float64) float64 { return quantile(v, 0.5) }

// quantile is the q-quantile of v by linear interpolation; 0 for no values.
func quantile(v []float64, q float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	pos := q * float64(len(s)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return s[lo]
	}
	return s[lo] + (s[hi]-s[lo])*(pos-float64(lo))
}

// WriteText renders the calibration for people, ending with the [lint.evals.estimate]
// snippet of the group with the most samples.
func (c *Calibration) WriteText(w io.Writer) error {
	var b strings.Builder
	if len(c.Groups) == 0 {
		b.WriteString("No recorded run has a token split to calibrate against.\n")
		b.WriteString("Run `ai-rulez eval run` (or --mode activation --surface native) with a runner that reports tokens first.\n")
		c.writeSkips(&b)
		_, err := io.WriteString(w, b.String())
		return err
	}
	best := 0
	for i := range c.Groups {
		g := &c.Groups[i]
		if g.Samples > c.Groups[best].Samples {
			best = i
		}
		fmt.Fprintf(&b, "%s, harness %s, model %s: %d run(s)%s\n", g.Kind, orDefault(g.Harness), orDefault(g.Model), g.Samples, lowNote(*g))
		fmt.Fprintf(&b, "  overhead_tokens          %6d -> %d\n", g.Current.OverheadTokens, g.Proposed.OverheadTokens)
		if g.Kind == KindActivation {
			fmt.Fprintf(&b, "  activation_output_tokens %6d -> %d\n", g.Current.ActivationOutputTokens, g.Proposed.ActivationOutputTokens)
		} else {
			fmt.Fprintf(&b, "  assumed_output_tokens    %6d -> %d\n", g.Current.AssumedOutputTokens, g.Proposed.AssumedOutputTokens)
		}
		fmt.Fprintf(&b, "  token error (median)     %+5.0f%% -> %+.0f%%; recorded cost error median %.0f%%, p90 %.0f%%\n",
			g.TokenErrorBefore*100, g.TokenErrorAfter*100, g.CostErrorMedian*100, g.CostErrorP90*100)
		if priceDiffers(*g) {
			fmt.Fprintf(&b, "  input price per MTok     %s list -> %s billed (prompt caching)\n", formatPrice(g.ListPriceIn), formatPrice(g.EffectivePriceIn))
		}
	}
	c.writeSkips(&b)
	g := c.Groups[best]
	fmt.Fprintf(&b, "\nProposed for %s, harness %s, model %s (add to .ai-rulez/config.toml or your user config):\n\n[lint.evals.estimate]\n", g.Kind, orDefault(g.Harness), orDefault(g.Model))
	fmt.Fprintf(&b, "overhead_tokens = %d\n", g.Proposed.OverheadTokens)
	if g.Kind == KindActivation {
		fmt.Fprintf(&b, "activation_output_tokens = %d\n", g.Proposed.ActivationOutputTokens)
	} else {
		fmt.Fprintf(&b, "assumed_output_tokens = %d\n", g.Proposed.AssumedOutputTokens)
	}
	if priceDiffers(g) {
		fmt.Fprintf(&b, "price_in_per_mtok = %s  # list price %s: the harness bills cached input at a discount\n", strconv.FormatFloat(g.EffectivePriceIn, 'f', -1, 64), formatPrice(g.ListPriceIn))
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// priceDiffers says the billed input price is far enough from the list price to propose.
func priceDiffers(g CalibrationGroup) bool {
	return g.ListPriceIn > 0 && g.EffectivePriceIn > 0 && math.Abs(g.EffectivePriceIn/g.ListPriceIn-1) > 0.10
}

func formatPrice(v float64) string { return "$" + strconv.FormatFloat(v, 'f', -1, 64) }

func (c *Calibration) writeSkips(b *strings.Builder) {
	s := c.Skipped
	if s.NoUsage+s.Unverified+s.Filtered == 0 {
		return
	}
	fmt.Fprintf(b, "left out: %d without a token split, %d unverified, %d for another harness or model\n", s.NoUsage, s.Unverified, s.Filtered)
}

func orDefault(s string) string {
	if s == "" {
		return "(default)"
	}
	return s
}

func lowNote(g CalibrationGroup) string {
	if g.LowConfidence {
		return " (low confidence: too few runs to trust)"
	}
	return ""
}
