package commands

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/verifiers"
)

// verifierFindingsFor runs the verifiers of cfg for `validate --strict
// --verifiers` and converts what did not pass into AR9H findings, so one
// invocation and one report cover lint and verification. It never starts a
// command (a command verifier is reported as skipped, AR9H3 at info) and never
// calls a model (an llm verifier is AR9H4, info). The whole repository is
// evaluated; `--since` of validate only narrows which findings are shown.
func verifierFindingsFor(ctx context.Context, cfg *config.Config) []lint.VerifierFinding {
	report := verifiers.Run(ctx, cfg, verifiers.Options{})
	if report.Err != nil {
		logger.Warn("Skipped the verifiers", "error", report.Err)
		return nil
	}
	var out []lint.VerifierFinding
	for i := range report.Results {
		out = append(out, verifierFindingsOf(cfg, &report.Results[i])...)
	}
	return out
}

// verifierFindingsOf converts one result; a pass, a not-applicable and an
// inactive verifier produce nothing.
func verifierFindingsOf(cfg *config.Config, res *verifiers.Result) []lint.VerifierFinding {
	switch res.Status {
	case verifiers.StatusFail, verifiers.StatusError, verifiers.StatusSkipped:
	default:
		return nil
	}
	code := res.Code
	if code == "" {
		code = verifiers.CodeVerifierFailed // a flat verifier that failed has no code of its own
	}
	if res.Status == verifiers.StatusError && res.Code == "" {
		code = verifiers.CodeVerifierInvalid // it could not be evaluated
	}
	sev := lint.Severity(res.Severity)
	switch {
	case res.Status == verifiers.StatusSkipped, code == verifiers.CodeVerifierCommand:
		sev = lint.SeverityInfo // validate never runs a command or a model: say so, do not fail
	case res.Status == verifiers.StatusError:
		sev = lint.SeverityError
	}
	head := res.Name + ": " + res.Message
	if res.Target != nil {
		head += " (" + res.Target.Kind + " " + res.Target.ID + ")"
	}
	decl := declarationFile(cfg, res)
	findings := res.Findings
	if res.Status != verifiers.StatusFail || len(findings) == 0 {
		return []lint.VerifierFinding{{Code: code, Severity: sev, File: decl, Line: 1, Message: head}}
	}
	out := make([]lint.VerifierFinding, 0, len(findings))
	for _, f := range findings {
		file := decl
		if f.File != "" {
			file = filepath.Join(cfg.BaseDir, filepath.FromSlash(f.File))
		}
		msg := res.Name + ": " + f.Message
		if res.Fix != "" {
			msg += " (fix: " + res.Fix + ")"
		}
		out = append(out, lint.VerifierFinding{Code: code, Severity: sev, File: file, Line: f.Line, Message: msg})
	}
	return out
}

// declarationFile is where a verifier is declared: its spec file, else config.toml.
func declarationFile(cfg *config.Config, res *verifiers.Result) string {
	if res.Source != "" && res.Source != "config.toml" && !strings.HasPrefix(res.Source, "include:") {
		if filepath.IsAbs(res.Source) {
			return res.Source
		}
		return filepath.Join(cfg.BaseDir, filepath.FromSlash(res.Source))
	}
	return filepath.Join(cfg.ConfigDir, "config.toml")
}
