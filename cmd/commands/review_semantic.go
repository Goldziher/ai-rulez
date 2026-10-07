package commands

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/samber/oops"
	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
	rv "github.com/Goldziher/ai-rulez/v5/internal/review"
)

// reviewClientFactory builds the model client of a judged run. Tests replace it with a client
// whose backend is a llm.Fake behind the real middleware (budget, cache, gate).
var reviewClientFactory = func(lc llm.Config, opts llm.Options) (llm.Client, error) {
	m, err := llm.New(lc, opts)
	if err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	return m, nil
}

// reviewNow is the clock of calibration ages (replaced in tests).
var reviewNow = time.Now

// tighterFloat is the lower positive cap of a and b; a non-finite value counts as unset, so a NaN
// cannot win the min (min(x, NaN) is NaN) and switch the cap off.
func tighterFloat(a, b float64) float64 {
	if !llm.Finite(a) {
		a = 0
	}
	if !llm.Finite(b) {
		b = 0
	}
	switch {
	case a <= 0:
		return b
	case b <= 0:
		return a
	default:
		return min(a, b)
	}
}

// checkCaps refuses a negative or non-finite --max-cost and a negative --max-calls or --k. A NaN
// passes `< 0` and then compares false with every spend, which would lift the cap.
func checkCaps(maxCost float64, maxCalls, k int) error {
	if !llm.Finite(maxCost) || maxCost < 0 || maxCalls < 0 || k < 0 {
		return oops.Errorf("--max-cost must be a finite number, and --max-cost, --max-calls and --k must not be negative")
	}
	return nil
}

func tighterInt(a, b int) int {
	switch {
	case a <= 0:
		return b
	case b <= 0:
		return a
	default:
		return min(a, b)
	}
}

// judgeSetup is the checked, ready-to-use model configuration of a judged run.
type judgeSetup struct {
	lc       llm.Config
	resolved config.LLMResolution
	review   config.ReviewResolution
	maxCost  float64
	maxCalls int
}

// resolveJudge resolves the model configuration and the spend caps of a judged run. It makes
// no check and no call: ready decides whether a call may be made.
func resolveJudge(cmd *cobra.Command, cfg *config.Config, model string, defCost float64, defCalls int) (*judgeSetup, error) {
	resolved, err := cfg.ResolveLLM(nil)
	if err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	maxCost, maxCalls, rres, err := reviewCaps(cmd, cfg, defCost, defCalls)
	if err != nil {
		return nil, err
	}
	lc := withModel(resolved.Config, model)
	if len(resolved.Ignored) > 0 {
		fmtWarn(cmd, llm.IgnoredKeysMessage(resolved.Ignored))
	}
	if len(rres.IgnoredRepoKeys) > 0 {
		fmtWarn(cmd, "repository [review] keys ignored (user scope only: set them in the user config file or AI_RULEZ_REVIEW_*): "+strings.Join(rres.IgnoredRepoKeys, ", "))
	}
	if host := llm.Diagnose(lc, llm.Options{ConfigDir: cfg.ConfigDir}).BaseURLHost; host != "" && len(rres.AllowedHosts) == 0 {
		fmtWarn(cmd, "[review] allowed_hosts is empty, so the judge may send item text to any host: "+host+" (set allowed_hosts in the user config file to pin the endpoint)")
	}
	lc.MaxCostUSD = tighterFloat(lc.MaxCostUSD, maxCost)
	lc.MaxCalls = tighterInt(lc.MaxCalls, maxCalls)
	return &judgeSetup{lc: lc, resolved: resolved, review: rres, maxCost: maxCost, maxCalls: maxCalls}, nil
}

// share returns the caps of one of n models judging the same items: the run's caps (the tighter of
// [llm] and the review caps) split evenly, so the n clients together stay within them. 0 stays
// unlimited.
func (js *judgeSetup) share(n int) (cost float64, calls int) {
	cost = tighterFloat(js.resolved.Config.MaxCostUSD, js.maxCost)
	calls = tighterInt(js.resolved.Config.MaxCalls, js.maxCalls)
	if n > 1 {
		cost /= float64(n)
		if calls > 0 {
			calls = max(calls/n, 1)
		}
	}
	return cost, calls
}

// ready checks that a call may be made: a model, the network opt-in in user scope, no
// organization policy against it, and a host the user's allow-list accepts.
func (js *judgeSetup) ready(cfg *config.Config) error {
	lc := js.lc
	switch {
	case lc.FullModel() == "":
		return oops.Hint("set [llm] model in config.toml, or pass --model").Errorf("no model configured for the judge")
	case config.PolicyLocksIn(activePolicy, "llm", cfg.BaseDir):
		return oops.Errorf("the organization policy forbids model calls ([llm] allow_network = false): a judged review is refused")
	case !lc.AllowNetwork:
		return oops.Hint("--semantic and allow_network are both required").Errorf("%s", llm.NetworkDisabledMessage)
	}
	host := llm.Diagnose(lc, llm.Options{ConfigDir: cfg.ConfigDir}).BaseURLHost
	if !js.review.HostAllowed(host) {
		shown := host
		if shown == "" {
			shown = config.ReviewProviderDefaultHost
		}
		return oops.Errorf("the model endpoint %s is not in [review] allowed_hosts (%s): a judged review is refused", shown, strings.Join(js.review.AllowedHosts, ", "))
	}
	return nil
}

func fmtWarn(cmd *cobra.Command, msg string) {
	fmt.Fprintln(cmd.ErrOrStderr(), "warning: "+msg) //nolint:errcheck // a terminal
}

// clientFor builds the client of a judge: the model layer with the spend caps applied.
func (js *judgeSetup) clientFor(cfg *config.Config, lc llm.Config) (llm.Client, error) {
	return reviewClientFactory(lc, llm.Options{ConfigDir: cfg.ConfigDir, CacheDir: reviewFlags.cacheDir, NoCache: reviewFlags.noCache})
}

// effectiveK is the vote cap of the run.
func effectiveK(rb *rv.Rubric, flag int) int {
	if flag > 0 {
		return flag
	}
	return max(rb.Votes.Max, 1)
}

// applyBaseline loads --baseline and hides the findings it lists.
func applyBaseline(res *rv.Results, _ *rv.Rubric) error {
	if reviewFlags.baseline == "" {
		return nil
	}
	b, err := rv.LoadBaseline(reviewFlags.baseline)
	if err != nil {
		return err //nolint:wrapcheck // already contextual
	}
	res.SetBaseline(b.Set())
	return nil
}

// finishBaseline records the baseline in the report and writes --write-baseline.
func finishBaseline(report *rv.Report, res *rv.Results, rb *rv.Rubric) error {
	if reviewFlags.baseline != "" {
		hidden, fresh := res.BaselineCounts(rb)
		report.Baseline = &rv.BaselineInfo{File: reviewFlags.baseline, Baselined: hidden, New: fresh}
		report.Summary.Baselined = hidden
	}
	if reviewFlags.writeBaseline != "" {
		return rv.WriteBaseline(reviewFlags.writeBaseline, report) //nolint:wrapcheck // already contextual
	}
	return nil
}

// semanticPlan is what a judged run settles before any call: the judge, its estimate and the caps.
type semanticPlan struct {
	js        *judgeSetup
	est       *rv.Estimate
	models    []string
	k         int
	gateLevel string
}

// planSemantic resolves the judge and refuses the run when the estimate or the caps say so.
func planSemantic(cmd *cobra.Command, rc *reviewContext, res *rv.Results) (*semanticPlan, error) {
	cfg, rb := rc.cfg, rc.rb
	models := reviewModelList()
	primary := reviewFlags.model
	if len(models) > 0 {
		primary = models[0]
	}
	// The plan decides whether a call is needed at all and whether the caps refuse the run.
	js, err := resolveJudge(cmd, cfg, primary, rv.DefaultMaxCostUSD, rv.DefaultMaxCalls)
	if err != nil {
		return nil, err
	}
	est := planWith(rc, res, js.lc, js.resolved, js.maxCost, js.maxCalls, false)
	if len(est.Refused) > 0 {
		return nil, oops.Errorf("the judged run is refused: %s", strings.Join(est.Refused, "; "))
	}
	if est.Totals.CallsMin > 0 {
		if err := js.ready(cfg); err != nil {
			return nil, err
		}
	}
	if len(models) > 1 {
		// The primary model is one of the compared models and gets its share, not the whole cap.
		js.lc.MaxCostUSD, js.lc.MaxCalls = js.share(len(models))
	}
	gateLevel := reviewFlags.gateLevel
	if gateLevel == "" {
		gateLevel = cfg.Review.GateLevel()
	}
	return &semanticPlan{js: js, est: est, models: models, k: effectiveK(rb, reviewFlags.k), gateLevel: gateLevel}, nil
}

// judgeCalibration is the calibration record of a judged run and how it matches the judge.
type judgeCalibration struct {
	rec    *rv.CalibrationRecord
	recErr error
	cur    rv.CalKey
	maxAge int
	now    time.Time
	alias  bool
	pre    rv.CalStatus
}

// loadJudgeCalibration matches the calibration record against the judge before any money is spent.
func loadJudgeCalibration(cfg *config.Config, rb *rv.Rubric, content string, js *judgeSetup, k int) *judgeCalibration {
	rec, recErr := rv.LoadCalibration(rv.CalibrationPath(cfg.ConfigDir, rb))
	cur := rv.CalKey{PromptDigest: rv.PromptDigest(rb), Content: content, K: k, Model: js.lc.FullModel()}
	if rb.Dir != "" {
		if set, gerr := rv.LoadGolden(rb.Dir, rb); gerr == nil {
			cur.GoldenDigest = set.Digest
		}
	}
	jc := &judgeCalibration{rec: rec, recErr: recErr, cur: cur, now: reviewNow(), alias: rv.IsFloatingAlias(js.lc.FullModel())}
	if cfg.Review != nil && cfg.Review.Gate != nil {
		jc.maxAge = cfg.Review.Gate.CalibrationMaxAgeDays
	}
	// The record holds the model id the provider reported (often dated), the request the id asked for:
	// when the record's id extends the requested one the model is compared after the first call.
	preKey := cur
	if rec != nil && strings.HasPrefix(rv.TrimModel(rec.Model), rv.TrimModel(cur.Model)+"-") {
		preKey.Model = ""
	}
	jc.pre = rv.MatchCalibration(rb, rec, preKey, jc.now, jc.maxAge)
	if recErr != nil {
		jc.pre = rv.CalStatus{State: rv.CalStale, Reasons: []string{"the calibration record cannot be read: " + recErr.Error()}}
	}
	return jc
}

// refuseGate refuses --gate before any call when the judge is a floating alias or not calibrated.
func (jc *judgeCalibration) refuseGate(cfg *config.Config, model string) error {
	switch {
	case jc.alias:
		return oops.Errorf("--gate is refused: model %s is a floating alias that may change under you; pin a model id (AR9G9)", model)
	case cfg.Review.GateRequiresCalibration() && jc.pre.State != rv.CalMatched:
		return oops.Hint("run `ai-rulez review calibrate` for this rubric, model and content mode").
			Errorf("--gate is refused: calibration %s: %s (AR9G9)", jc.pre.State, strings.Join(jc.pre.Reasons, "; "))
	}
	return nil
}

// settle matches the calibration again with the model id the provider reported: a model that
// answers under another id than the record was calibrated for is not that judge.
func (jc *judgeCalibration) settle(rb *rv.Rubric, outcome *rv.SemanticOutcome, requested string) (cal rv.CalStatus, resolved []string) {
	resolved = outcome.Usage.ResolvedModels()
	if len(resolved) > 0 {
		jc.cur.Model = resolved[0]
	}
	cal = rv.MatchCalibration(rb, jc.rec, jc.cur, jc.now, jc.maxAge)
	if jc.recErr != nil {
		cal = jc.pre
	}
	if len(resolved) > 0 && !rv.SameModel(resolved[0], requested) {
		jc.alias = true
	}
	return cal, resolved
}

// evaluateGate decides --gate over the judged items, or records why the run cannot vouch for them.
func evaluateGate(cfg *config.Config, res *rv.Results, outcome *rv.SemanticOutcome, cal rv.CalStatus, gate *rv.GateResult) {
	switch {
	case outcome.Incomplete:
		gate.Refused = "the run is incomplete (" + outcome.StoppedBecause + "), so it cannot vouch for the items it did not judge"
	case len(outcome.Truncated) > 0:
		gate.Refused = "the judge saw only part of " + strings.Join(outcome.Truncated, ", ") + " (the body was truncated), so it cannot vouch for them; shorten the item or raise max_item_tokens"
	case cfg.Review.GateRequiresCalibration() && cal.State != rv.CalMatched:
		gate.Refused = "calibration " + cal.State + ": " + strings.Join(cal.Reasons, "; ")
	default:
		var calibrated map[string]bool
		if cfg.Review.GateRequiresCalibration() {
			calibrated = map[string]bool{}
			for _, d := range cal.Dimensions {
				calibrated[d] = true
			}
		}
		*gate = rv.EvaluateGate(res, gate.Level, calibrated)
	}
}

// runSemantic runs the judged review and writes the report.
func runSemantic(cmd *cobra.Command, rc *reviewContext, res *rv.Results, out io.Writer) (int, error) {
	cfg, rb := rc.cfg, rc.rb
	sp, err := planSemantic(cmd, rc, res)
	if err != nil {
		return exitReviewRefused, err
	}
	js, est := sp.js, sp.est
	if err := applyBaseline(res, rb); err != nil {
		return exitReviewRefused, err
	}

	// Calibration is checked before any money is spent, so a refused gate costs nothing.
	jc := loadJudgeCalibration(cfg, rb, rc.content(), js, sp.k)
	gate := &rv.GateResult{Requested: reviewFlags.gate, Level: sp.gateLevel}
	if reviewFlags.gate {
		if err := jc.refuseGate(cfg, js.lc.FullModel()); err != nil {
			return exitReviewRefused, err
		}
	}

	outcome := &rv.SemanticOutcome{}
	if est.Totals.CallsMin > 0 {
		if outcome, err = judgeWith(cmd, cfg, js, js.lc, rb, res, sp.k, rc.content()); err != nil {
			return exitReviewRefused, err
		}
	}

	cal, resolved := jc.settle(rb, outcome, js.lc.FullModel())
	if reviewFlags.gate {
		evaluateGate(cfg, res, outcome, cal, gate)
	}
	addRunNotes(res, rb, cfg, outcome, cal, jc.alias, js.lc.FullModel(), resolved)

	var comparison *rv.ModelComparison
	if len(sp.models) > 1 && est.Totals.CallsMin > 0 {
		var extra rv.RunUsage
		comparison, extra, err = compareModels(cmd, cfg, js, rb, res, sp.models, sp.k, rc)
		if err != nil {
			return exitReviewRefused, err
		}
		outcome.Usage.Add(extra)
	}

	if err := writeSemanticReport(out, rc, res, sp, outcome, cal, gate, comparison); err != nil {
		return exitReviewRefused, err
	}
	return gateExit(gate, outcome), nil
}

// gateExit is the exit status of a judged run: refused or incomplete under --gate, or a failed gate.
func gateExit(gate *rv.GateResult, outcome *rv.SemanticOutcome) int {
	switch {
	case gate.Requested && (gate.Refused != "" || outcome.Incomplete):
		return exitReviewRefused
	case gate.Requested && !gate.Passed:
		return exitReviewGate
	}
	return 0
}

// writeSemanticReport builds the report of a judged run, settles the baseline and writes it.
func writeSemanticReport(out io.Writer, rc *reviewContext, res *rv.Results, sp *semanticPlan, outcome *rv.SemanticOutcome, cal rv.CalStatus, gate *rv.GateResult, comparison *rv.ModelComparison) error {
	js, rb := sp.js, rc.rb
	report := rv.NewReport(rb, res, nil)
	var reported *rv.GateResult
	if gate.Requested {
		reported = gate
	}
	report.WithSemantic(rv.SemanticReport{
		Outcome: outcome, Model: js.lc.FullModel(), Votes: sp.k, Content: rc.content(), CapUSD: js.maxCost, CapCalls: js.maxCalls,
		Host: hostOf(js.lc, rc.cfg.ConfigDir), Calibration: &cal, Gate: reported, Models: comparison, Estimate: sp.est,
	})
	if err := finishBaseline(report, res, rb); err != nil {
		return err
	}
	return writeReview(out, report, rb, res)
}

// judgeWith builds the client for lc and judges every scored item of res.
func judgeWith(cmd *cobra.Command, cfg *config.Config, js *judgeSetup, lc llm.Config, rb *rv.Rubric, res *rv.Results, k int, content string) (*rv.SemanticOutcome, error) {
	client, err := js.clientFor(cfg, lc)
	if err != nil {
		return nil, err
	}
	defer client.Close() //nolint:errcheck // nothing to flush
	outcome, err := rv.RunSemantic(commandContext(cmd), rv.SemanticInput{
		Rubric: rb, Results: res, Options: rv.SemanticOptions{Client: client, K: k, Content: content, Workers: reviewFlags.concurrency},
	})
	if err != nil {
		if errors.Is(err, llm.ErrNetworkDisabled) {
			return nil, oops.Hint("set [llm] allow_network = true in your user config").Errorf("%s", llm.NetworkDisabledMessage)
		}
		return nil, oops.Wrapf(err, "the judged run stopped")
	}
	return outcome, nil
}

// compareModels judges the items again with each further model and compares the verdicts.
func compareModels(cmd *cobra.Command, cfg *config.Config, js *judgeSetup, rb *rv.Rubric, res *rv.Results, models []string, k int, rc *reviewContext) (*rv.ModelComparison, rv.RunUsage, error) {
	var used rv.RunUsage
	names := []string{js.lc.FullModel()}
	tables := []map[string]map[string]string{rv.VerdictTable(res)}
	for _, m := range models[1:] {
		lc := withModel(js.resolved.Config, m)
		lc.MaxCostUSD, lc.MaxCalls = js.share(len(models))
		other := res.CloneUnjudged()
		out, err := judgeWith(cmd, cfg, js, lc, rb, other, k, rc.content())
		if err != nil {
			return nil, used, err
		}
		used.Add(out.Usage)
		names = append(names, lc.FullModel())
		tables = append(tables, rv.VerdictTable(other))
	}
	dims := make([]string, 0, len(rb.Dimensions))
	for i := range rb.Dimensions {
		dims = append(dims, rb.Dimensions[i].ID)
	}
	return rv.CompareModels(names, tables, dims), used, nil
}

// addRunNotes adds the run-level findings: items left unjudged (AR9G0) and a judge that is not
// calibrated for this run (AR9G9).
func addRunNotes(res *rv.Results, rb *rv.Rubric, cfg *config.Config, outcome *rv.SemanticOutcome, cal rv.CalStatus, alias bool, requested string, resolved []string) {
	path := filepath.ToSlash(filepath.Join(filepath.Base(cfg.ConfigDir), "config.toml"))
	if rb.Dir != "" {
		path = filepath.ToSlash(filepath.Join(filepath.Base(cfg.ConfigDir), rv.RubricsDir, rb.Ref, rv.RubricFile))
	}
	for _, id := range outcome.Unjudged {
		res.AddNote(rv.Finding{
			Code: lint.CodeReviewRunNote, Name: "review-run-note", Severity: string(lint.SeverityInfo), ItemID: id, Path: path, Line: 1,
			Message: id + " was not judged: " + outcome.StoppedBecause, Origin: rv.OriginRun, Fingerprint: "run-unjudged-" + id,
		})
	}
	var why []string
	if alias {
		why = append(why, fmt.Sprintf("the model %s is a floating alias or answered under another id (%s): the calibration may not describe it", requested, strings.Join(resolved, ", ")))
	}
	if cal.State != rv.CalMatched {
		why = append(why, "calibration "+cal.State+": "+strings.Join(cal.Reasons, "; "))
	}
	if len(why) > 0 {
		res.AddNote(rv.Finding{
			Code: lint.CodeReviewCalibrationStale, Name: "judge-calibration-stale", Severity: string(lint.SeverityInfo), Path: path, Line: 1,
			Message: "advisory judge: " + strings.Join(why, "; "), Origin: rv.OriginRun, Fingerprint: "run-calibration",
		})
	}
}
