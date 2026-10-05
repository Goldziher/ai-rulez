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

func init() {
	for _, r := range okf.Rules() {
		registerRules(RuleInfo{Code: r.Code, Name: r.Name, Default: Severity(r.Default), Describe: r.Describe})
	}
}

// WithOKF supplies the OKF bundle directory and its findings (see
// okfbridge.CheckProject) to report as AR9B0-AR9B9.
func WithOKF(dir string, findings []okf.Finding) Option {
	return func(r *runner) {
		r.okfDir = dir
		r.okfFindings = findings
	}
}

func (r *runner) checkOKF() {
	for i := range r.okfFindings {
		f := r.okfFindings[i]
		// A finding may be milder than its rule's default (a log heading is a
		// warning under AR9B6). Honor that unless the config set a severity.
		code := f.Code
		saved, had := r.sev[code]
		if had && saved == Severity(okf.DefaultSeverity(code)) && Severity(f.Severity) != saved {
			r.sev[code] = Severity(f.Severity)
		}
		r.add(code, filepath.Join(r.okfDir, filepath.FromSlash(f.Path)), f.Line, "%s", f.Message)
		if had {
			r.sev[code] = saved
		}
	}
}
