package lint

import (
	"path/filepath"

	"github.com/Goldziher/ai-rulez/v5/internal/okf"
)

// The OKF family (AR9B0-AR9B9) lints the project's Open Knowledge Format bundle.
// The checks themselves live in internal/okf; this file registers the codes and
// turns a bundle's findings into strict-validation findings.

// Rule codes of the OKF family. They repeat the literals of internal/okf so the
// registry test can see them; TestOKFCodesMatchPackage keeps them in step.
const (
	CodeOKFIndexMismatch     = "AR9B0"
	CodeOKFTypeInvalid       = "AR9B1"
	CodeOKFLinkBroken        = "AR9B2"
	CodeOKFVersionInvalid    = "AR9B3"
	CodeOKFOrphan            = "AR9B4"
	CodeOKFExportDrift       = "AR9B5"
	CodeOKFReservedStructure = "AR9B6"
	CodeOKFTitleDuplicate    = "AR9B7"
	CodeOKFPathUnsafe        = "AR9B8"
	CodeOKFLossyMapping      = "AR9B9"
)

func registerOkf(s *ruleSet) {
	for _, r := range okf.Rules() {
		s.addRules(RuleInfo{Code: r.Code, Name: r.Name, Default: Severity(r.Default), Describe: r.Describe})
	}
}

// okfBundle is one OKF bundle directory and its findings.
type okfBundle struct {
	dir      string
	findings []okf.Finding
}

// WithOKF supplies an OKF bundle directory and its findings (see
// okfbridge.CheckProject) to report as AR9B0-AR9B9. It may be passed more than
// once: a project can have a configuration directory that is a bundle and a
// separate exported one.
func WithOKF(dir string, findings []okf.Finding) Option {
	return func(r *runner) {
		r.okfBundles = append(r.okfBundles, okfBundle{dir: dir, findings: findings})
	}
}

func (r *runner) checkOKF() {
	for _, b := range r.okfBundles {
		r.checkOKFBundle(b.dir, b.findings)
	}
}

func (r *runner) checkOKFBundle(dir string, findings []okf.Finding) {
	for i := range findings {
		f := findings[i]
		// A finding may be milder than its rule's default (a log heading is a
		// warning under AR9B6). Honor that unless the config set a severity.
		code := f.Code
		saved, had := r.sev[code]
		if had && saved == Severity(okf.DefaultSeverity(code)) && Severity(f.Severity) != saved {
			r.sev[code] = Severity(f.Severity)
		}
		r.add(code, filepath.Join(dir, filepath.FromSlash(f.Path)), f.Line, "%s", f.Message)
		if had {
			r.sev[code] = saved
		}
	}
}
