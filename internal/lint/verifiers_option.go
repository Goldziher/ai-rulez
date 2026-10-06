package lint

// VerifierFinding is a verifier result computed outside the lint package (the
// verifiers need the generator, which imports lint): `validate --strict
// --verifiers` evaluates them and hands over the findings, which are reported
// under the AR9H codes.
type VerifierFinding struct {
	Code string
	// Severity overrides the rule's default severity when set (a verifier's own
	// severity for AR9H1).
	Severity Severity
	// File is the absolute path the finding is shown at: the offending file, else
	// the declaration.
	File    string
	Line    int
	Message string
}

// WithVerifiers supplies the verifier findings computed by the caller.
func WithVerifiers(findings []VerifierFinding) Option {
	return func(r *runner) { r.verifiers = findings }
}

// checkVerifiers reports the supplied verifier findings.
func (r *runner) checkVerifiers() {
	for _, f := range r.verifiers {
		r.addWithSeverity(f.Code, f.Severity, f.File, f.Line, "%s", f.Message)
	}
}
