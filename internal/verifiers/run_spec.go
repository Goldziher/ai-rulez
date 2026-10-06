package verifiers

import (
	"context"
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
	defer func() { res.Message = sanitize(res.Message) }()
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
		return "paired"
	case r.GlobCount != nil:
		return config.VerifierGlobCount
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
