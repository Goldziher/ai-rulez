package lint

// Codes of lock attestations (docs/signing.md). The commands verify the
// attestation (internal/signing) and pass the failures in with WithSigning,
// because this package must not import the signing machinery. The same codes are
// declared in internal/signing; a test keeps the two equal.
const (
	CodeSignatureMissing   = "AR720"
	CodeSignatureInvalid   = "AR721"
	CodeSignerNotTrusted   = "AR722"
	CodeSignatureStale     = "AR723"
	CodeAttestationSubject = "AR724"
	CodeTrustedRootMissing = "AR725"
	CodeTLogProofMissing   = "AR726"
	CodeSignatureRollback  = "AR727"
)

// WithSigning supplies the signing findings to report (AR720 to AR727). They are
// reported like approval findings: one line per finding against the lock file.
// Apply it after WithApprovals so the two lists are joined, not replaced.
func WithSigning(findings []ApprovalFinding) Option {
	return func(r *runner) { r.approvals = append(r.approvals, findings...) }
}

func init() {
	registerRules(
		RuleInfo{CodeSignatureMissing, "signature-missing", SeverityError, "[signing] require asks for a signed lock and no attestation file exists"},
		RuleInfo{CodeSignatureInvalid, "signature-invalid", SeverityError, "the lock attestation is not a valid Sigstore bundle: bad envelope, signature, certificate chain or log proof"},
		RuleInfo{CodeSignerNotTrusted, "signer-not-trusted", SeverityError, "the lock was signed by an identity, issuer or key that no [[signing.trust]] entry accepts, or an identity_regexp is not anchored"},
		RuleInfo{CodeSignatureStale, "signature-stale", SeverityError, "the lock signature is older than [signing] max_age"},
		RuleInfo{CodeAttestationSubject, "attestation-subject-mismatch", SeverityError, "the signed digest or hash_version differs from the lock: the lock changed after it was signed"},
		RuleInfo{CodeTrustedRootMissing, "trusted-root-unavailable", SeverityError, "a certificate-signed attestation needs a Sigstore trusted root and none is configured or cached"},
		RuleInfo{CodeTLogProofMissing, "tlog-proof-missing", SeverityError, "[signing] tlog requires a transparency log entry and the bundle has none"},
		RuleInfo{CodeSignatureRollback, "signature-rollback", SeverityError, "the lock attestation is older than one this machine already verified for the repository"},
	)
	registerRuleDocs(map[string]RuleDoc{
		CodeSignatureMissing: {
			Why:  "[signing] require = [\"lock\"] says the committed lock must be signed, and there is no attestation file next to it.",
			Bad:  "`require = [\"lock\"]` and no `.ai-rulez/ai-rulez.lock.sigstore.json`",
			Good: "Run `ai-rulez sign --lock` in the release workflow and commit the bundle",
		},
		CodeSignatureInvalid: {
			Why:  "The bundle is truncated, edited, signed by a certificate the trusted root does not chain to, or carries a bad log proof.",
			Bad:  "A bundle whose payload was edited after signing",
			Good: "Sign again with `ai-rulez sign --lock`; verify with `ai-rulez verify --attestation`",
		},
		CodeSignerNotTrusted: {
			Why:  "A valid signature only says who signed. The identity and issuer (or key) must match a trust entry, exactly or by an anchored identity_regexp, and be inside its validity window.",
			Bad:  "A lock signed by a contributor's own GitHub identity when only the release workflow is trusted",
			Good: "Sign from the trusted workflow, or add a reviewed `[[signing.trust]]` entry",
		},
		CodeSignatureStale: {
			Why:  "max_age bounds how old an accepted signature may be, so a stale but valid lock cannot be replayed indefinitely.",
			Bad:  "`max_age = \"30d\"` and a signature from three months ago",
			Good: "Re-run `ai-rulez lock` and `ai-rulez sign --lock` on your release cadence",
		},
		CodeAttestationSubject: {
			Why:  "The attestation covers one lock-subject digest. Any change to the lock (or a different hash_version) makes it attest something else.",
			Bad:  "`ai-rulez lock` re-pinned a source after the lock was signed",
			Good: "Sign after the final `ai-rulez lock`",
		},
		CodeTrustedRootMissing: {
			Why:  "A keyless certificate is checked against the Sigstore trusted root. Verification is offline, so the root must be a file you pass or cache.",
			Bad:  "A keyless attestation and neither `trusted_root` nor a cached root",
			Good: "Run `ai-rulez trust update` once, or commit a trusted root and set `trusted_root`",
		},
		CodeTLogProofMissing: {
			Why:  "A short-lived certificate is only meaningful at the time the log recorded the signature. Without a log entry there is no trustworthy signing time.",
			Bad:  "A key bundle signed with `--no-tlog` under `tlog = \"required\"`",
			Good: "Sign with a log entry, or set `tlog = \"off\"` for a key-only setup",
		},
		CodeSignatureRollback: {
			Why:  "This machine has already verified a newer attestation for the repository. Accepting an older one would roll the lock back to a state that was valid once.",
			Bad:  "Presenting last quarter's signed lock after this quarter's was verified",
			Good: "Use the current attestation; the per-user high-water mark lives in the state directory (docs/signing.md)",
		},
	})
	for _, code := range []string{CodeSignatureMissing, CodeSignatureInvalid, CodeSignerNotTrusted, CodeSignatureStale, CodeAttestationSubject, CodeTrustedRootMissing, CodeTLogProofMissing, CodeSignatureRollback} {
		SetAnalyzer(code, AnalyzerLock, ScopeBundle)
	}
}
