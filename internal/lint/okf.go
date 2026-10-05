package lint

import (
	"path/filepath"

	"github.com/Goldziher/ai-rulez/internal/okf"
)

// The OKF family (AR9B0-AR9B9) lints the project's Open Knowledge Format bundle.
// The checks themselves live in internal/okf; this file registers the codes and
// turns a bundle's findings into strict-validation findings.

// Rule codes of the OKF family, re-exported for config and tests.
const (
	CodeOKFIndexMismatch     = okf.CodeIndexMismatch
	CodeOKFTypeInvalid       = okf.CodeTypeInvalid
	CodeOKFLinkBroken        = okf.CodeLinkBroken
	CodeOKFVersionInvalid    = okf.CodeVersionInvalid
	CodeOKFOrphan            = okf.CodeOrphan
	CodeOKFExportDrift       = okf.CodeExportDrift
	CodeOKFReservedStructure = okf.CodeReservedStructure
	CodeOKFTitleDuplicate    = okf.CodeTitleDuplicate
	CodeOKFPathUnsafe        = okf.CodePathUnsafe
	CodeOKFLossyMapping      = okf.CodeLossyMapping
)

func init() {
	for _, r := range okf.Rules() {
		registry = append(registry, RuleInfo{Code: r.Code, Name: r.Name, Default: Severity(r.Default), Describe: r.Describe})
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
