package verifiers

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// evaluateSpec runs one spec verifier against the prepared scope.
func evaluateSpec(ctx context.Context, env *Env, sp *Spec) (res Result) {
	res = Result{
		Name: sp.ID, Type: predicateKind(sp.Require), Severity: sp.Severity, Description: sp.Description,
		Fix: sp.Fix, Source: sp.source, Target: resolveTarget(env.Cfg, sp),
	}
	if res.Severity == "" {
		res.Severity = severityWarning
	}
	advisory := usesLLM(sp.Require)
	defer func() {
		res.Message = sanitize(res.Message)
		if advisory {
			capAdvisory(env, sp, &res)
		}
	}()
	if err := ctx.Err(); err != nil {
		res.Status, res.Message = StatusError, "not run: "+err.Error()
		return res
	}
	scoped, applicable, err := env.scopedFiles(sp)
	if err != nil {
		res.Status, res.Code, res.Message = StatusError, CodeVerifierInvalid, err.Error()
		return res
	}
	if !applicable {
		return notApplicable(env, sp, res)
	}
	c := &evalCtx{env: env, spec: sp, scoped: scoped}
	out, err := c.eval(ctx, sp.Require)
	res.Notes = c.notes
	switch {
	case err != nil:
		res.Status, res.Message = StatusError, err.Error()
		var ce *codedError
		if errors.As(err, &ce) {
			res.Status, res.Code = ce.status, ce.code
		}
	case out.pass:
		res.Status = StatusPass
		if len(sp.WhenChanged) > 0 {
			res.Message = fmt.Sprintf("%d file(s)", len(scoped))
		}
	default:
		sortFindings(out.findings)
		res.Status, res.Code, res.Findings = StatusFail, CodeVerifierFailed, out.findings
		res.Message = sp.Message
		if res.Message == "" {
			res.Message = summarize(out.findings)
		}
	}
	return res
}

func notApplicable(env *Env, sp *Spec, res Result) Result {
	if env.opts.StrictApplicability && env.deadScope(sp) {
		res.Status, res.Code, res.Severity = StatusFail, CodeVerifierDeadScope, severityWarning
		res.Message = "when_changed matches no file in the repository: " + strings.Join(sp.WhenChanged, ", ")
		res.Fix = "Fix the glob or delete the verifier."
		return res
	}
	res.Status, res.Message = StatusNotApplicable, "no changed file matches when_changed"
	return res
}

func summarize(findings []Finding) string {
	if len(findings) == 0 {
		return "the predicate does not hold"
	}
	f := findings[0]
	msg := f.Message
	if f.File != "" {
		msg = f.File + ": " + msg
	}
	if len(findings) > 1 {
		msg += fmt.Sprintf(" (and %d more)", len(findings)-1)
	}
	return msg
}

// predicateKind names the root predicate of a require tree.
func predicateKind(r *Require) string {
	switch {
	case r == nil:
		return ""
	case r.Regex != nil:
		return config.VerifierRegex
	case r.Forbid != nil:
		return config.VerifierForbid
	case r.FileExists != nil:
		return config.VerifierFileExists
	case r.Paired != nil:
		return predicatePaired
	case r.GlobCount != nil:
		return config.VerifierGlobCount
	case r.Command != nil:
		return predicateCommand
	case r.LLM != nil:
		return "llm"
	case len(r.All) > 0:
		return "all"
	case len(r.Any) > 0:
		return "any"
	}
	return "not"
}

// resolveTarget finds the file (and heading line) of the item a spec enforces.
func resolveTarget(cfg *config.Config, sp *Spec) *Target {
	kind, id := sp.TargetKind()
	t := &Target{Kind: kind, ID: id}
	if cfg.Content == nil {
		return t
	}
	cf, ok := findTarget(cfg, kind, id)
	if !ok {
		return t
	}
	t.Path = relTo(cfg.BaseDir, cf.Path)
	if sp.Anchor != "" {
		t.Line = anchorLine(cf.Content, sp.Anchor)
	}
	return t
}

// inactiveResult is a verifier whose target is outside the active profile or role.
func inactiveResult(env *Env, sp *Spec, why string) Result {
	res := Result{
		Name: sp.ID, Type: predicateKind(sp.Require), Severity: sp.Severity, Description: sp.Description,
		Source: sp.source, Target: resolveTarget(env.Cfg, sp), Status: StatusInactive, Message: why,
	}
	if res.Severity == "" {
		res.Severity = severityWarning
	}
	return res
}

// missingExamples reports a spec without self-test examples when
// [verifiers_settings] require_examples is set (AR9H6).
func missingExamples(env *Env, sp *Spec) (Result, bool) {
	if s := env.settings(); !s.RequireExamples || len(sp.Examples) > 0 {
		return Result{}, false
	}
	return Result{
		Name: sp.ID, Type: "examples", Severity: severityWarning, Status: StatusFail, Code: CodeVerifierNoExamples,
		Message: "no self-test examples", Source: sp.source, Target: resolveTarget(env.Cfg, sp),
		Fix:      "Add [[verifiers.examples]] with a passing and a failing case, then run `ai-rulez verifiers test " + sp.ID + "`.",
		Findings: []Finding{{Message: "verifier " + sp.ID + " has no examples"}},
	}, true
}

// capAdvisory marks the result of a verifier that asks a model: its severity
// never exceeds warning (a model's verdict is advisory), and a skipped one is
// info (AR9H4).
//
// A failing verifier declared at error severity keeps it only under --gate-llm
// and only while its calibration record is current and meets the bar
// (CalibrationMinPrecision on labeled examples); see Calibrate.
func capAdvisory(env *Env, sp *Spec, res *Result) {
	res.Advisory = true
	switch {
	case res.Status == StatusSkipped:
		res.Severity = severityInfo
	case res.Severity == severityError:
		if res.Status == StatusFail && env.opts.LLM != nil && env.opts.LLM.Gate {
			rec, err := LoadCalibration(env.Cfg, sp.ID)
			ok, why := false, ""
			if err != nil {
				why = "its calibration record cannot be read: " + err.Error()
			} else {
				ok, why = calibrationVerdict(rec, sp, effectiveModel(sp, env.opts.LLM.Model))
			}
			if ok {
				res.Advisory = false
				res.Notes = append(res.Notes, gateNote(rec))
				return
			}
			res.Severity = severityWarning
			res.Notes = append(res.Notes, "advisory: not gating, "+why)
			return
		}
		res.Severity = severityWarning
		res.Notes = append(res.Notes, "advisory: the severity of an llm verifier is capped at warning (--gate-llm lets a calibrated verifier fail the run)")
	}
}
