package lint

import "path/filepath"

// Codes of reviewer approvals (docs/approvals.md). The commands compute the
// status of every pinned item from [governance] and the [[approval]] records of
// ai-rulez.lock (internal/approval) and pass the failures in with WithApprovals,
// because this package must not import the lock machinery. The same codes are
// declared in internal/approval; a test keeps the two equal.
const (
	CodeApprovalMissing      = "AR710"
	CodeApprovalStale        = "AR711"
	CodeApprovalExpired      = "AR712"
	CodeApproverUnauthorized = "AR713"
	CodeApprovalInsufficient = "AR714"
	CodeApprovalOrphan       = "AR715"
	CodeApprovalSelf         = "AR716"
	CodeApprovalDenied       = "AR717"
	CodeApprovalUnverified   = "AR718"
	CodeApproverUnresolved   = "AR719"
)

// ApprovalFinding is one approval problem: Code is one of the codes above and
// Path the file the finding is shown against (the lock, relative to the repo).
type ApprovalFinding struct {
	Code    string
	Path    string
	Message string
}

// WithApprovals supplies the approval findings to report (AR710 to AR719).
func WithApprovals(findings []ApprovalFinding) Option {
	return func(r *runner) { r.approvals = findings }
}

func init() {
	registerRules(
		RuleInfo{CodeApprovalMissing, "approval-missing", SeverityError, "content that [governance] require_approval selects has no reviewer approval in ai-rulez.lock"},
		RuleInfo{CodeApprovalStale, "approval-stale", SeverityError, "content was approved, but its digest changed since: the approval no longer applies"},
		RuleInfo{CodeApprovalExpired, "approval-expired", SeverityError, "every approval of the current digest is past its expiry date"},
		RuleInfo{CodeApproverUnauthorized, "approver-not-authorized", SeverityError, "the current digest is approved only by reviewers outside [governance] approvers"},
		RuleInfo{CodeApprovalInsufficient, "approval-insufficient", SeverityError, "fewer distinct reviewers approved the current digest than [governance] min_approvers asks for"},
		RuleInfo{CodeApprovalSelf, "approval-with-change", SeverityError, "an approval was added in the same change as the content it approves (found only with --approvals-base or `approve --verify-base`)"},
		RuleInfo{CodeApprovalDenied, "approval-denied", SeverityError, "content's digest is on the deny list in ai-rulez.lock: it can be neither approved nor used"},
		RuleInfo{CodeApprovalUnverified, "approval-unverified", SeverityError, "every approval of the current digest claims an assurance (signed, review-linked) that could not be verified"},
		RuleInfo{CodeApproverUnresolved, "approver-unresolved", SeverityError, "[governance] approvers_from or a team cannot be resolved (no CODEOWNERS file, or a team with no member list), so nobody is authorized by it"},
		RuleInfo{CodeApprovalOrphan, "approval-orphan", SeverityWarning, "an approval in ai-rulez.lock names content that no longer exists; remove it with `ai-rulez approve --prune`"},
	)
	registerRuleDocs(map[string]RuleDoc{
		CodeApprovalMissing: {
			Why:  "[governance] require_approval says this content must be read and accepted by a person before agents use it, and the lock records no such decision.",
			Bad:  "An included skill pack with no `[[approval]]` entry while `require_approval = [\"remote\"]`",
			Good: "Read the content, then run `ai-rulez approve include:shared` and commit the lock",
		},
		CodeApprovalStale: {
			Why:  "An approval is bound to one content digest. New bytes, a changed script or a flipped executable bit are new content that nobody has reviewed, so the old approval stops applying.",
			Bad:  "A remote include moved to a new commit after `approve include:shared`",
			Good: "Review the change (`ai-rulez approve --diff include:shared`), then approve the new digest",
		},
		CodeApprovalExpired: {
			Why:  "An approval carries an expiry so a decision is not valid forever; after the date the content must be reviewed again.",
			Bad:  "`expires = \"2026-01-05\"` on the record, checked on a later date",
			Good: "Review the content again and run `ai-rulez approve` to record a new approval",
		},
		CodeApproverUnauthorized: {
			Why:  "[governance] approvers names who may approve; a record by anyone else does not count.",
			Bad:  "`approvers = [\"alice@example.org\"]` and the record's reviewer is `bob@example.org`",
			Good: "Have an allowed reviewer run `ai-rulez approve`, or add the reviewer to `approvers` through a reviewed change",
		},
		CodeApprovalInsufficient: {
			Why:  "min_approvers asks for several distinct reviewers; one person approving twice counts once.",
			Bad:  "`min_approvers = 2` with a single reviewer on record",
			Good: "Another reviewer runs `ai-rulez approve` for the same digest",
		},
		CodeApprovalSelf: {
			Why:  "An approval in the committed lock is an assertion, not authentication: whoever edits the lock can add one. When an approval arrives in the same change as the content it approves, nobody but the author vouched for it, so CI should demand a second reviewer.",
			Bad:  "A pull request that edits a skill's script and adds `[[approval]]` for the new digest",
			Good: "Land the content change first, then approve it in a separate, separately reviewed change",
		},
		CodeApprovalDenied: {
			Why:  "A `[[deny]]` entry names a digest that was found harmful. Approving it, or serving it after a re-pin, would reintroduce it, so the digest is refused whether or not [governance] selects the item.",
			Bad:  "A skill whose digest equals a `[[deny]]` entry after a downgrade to an old version",
			Good: "Remove or replace the content; `ai-rulez approve --revoke <item> --deny --reason ...` adds an entry",
		},
		CodeApprovalUnverified: {
			Why:  "A signed approval counts only when its attestation verifies against `[[signing.trust]]` entries with `subject = \"approval\"`; a review-linked one needs its `ref`. A record that fails this proves nothing about who reviewed.",
			Bad:  "`assurance = \"signed\"` with an attestation signed by a key no trust entry names",
			Good: "Have a trusted signer run `ai-rulez approve --sign`, or trust the signer in `[[signing.trust]]` through a reviewed change",
		},
		CodeApproverUnresolved: {
			Why:  "`approvers_from` restricts approval to the owners of an item's path. When the CODEOWNERS file is missing, or an owner is a team whose members are unknown, ai-rulez fails closed instead of letting anyone approve.",
			Bad:  "`approvers_from = \"CODEOWNERS\"` with no CODEOWNERS file, or `@acme/security` owning the lock with no `[governance.teams]` entry",
			Good: "Add the CODEOWNERS file, list the team's members in `[governance.teams]`, or resolve them with `approve --resolve-teams`",
		},
		CodeApprovalOrphan: {
			Why:  "The item an approval names was removed or renamed, so the record can never apply; a renamed item must be approved again under its new name.",
			Bad:  "An `[[approval]]` for a hook that no longer exists",
			Good: "Run `ai-rulez approve --prune` (or `ai-rulez lock`, which drops orphans) and commit the lock",
		},
	})
	for _, code := range []string{CodeApprovalMissing, CodeApprovalStale, CodeApprovalExpired, CodeApproverUnauthorized, CodeApprovalInsufficient, CodeApprovalOrphan, CodeApprovalSelf, CodeApprovalDenied, CodeApprovalUnverified, CodeApproverUnresolved} {
		SetAnalyzer(code, AnalyzerLock, ScopeBundle)
	}
	registerRunCheck((*runner).checkApprovals, AnalyzerLock)
}

// checkApprovals reports the supplied approval findings.
func (r *runner) checkApprovals() {
	for _, f := range r.approvals {
		abs := f.Path
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(r.rootAbs(), filepath.FromSlash(f.Path))
		}
		r.add(f.Code, abs, 1, "%s", f.Message)
	}
}
