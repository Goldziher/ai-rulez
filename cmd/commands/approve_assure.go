package commands

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/approval"
	"github.com/Goldziher/ai-rulez/v5/internal/forge"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/signing"
)

// approvalDraft is one approval record about to be written, with the bundle
// that backs it when it is signed.
type approvalDraft struct {
	reviewer    string
	assurance   string
	ref         string
	attestation string
	bundle      []byte
	// skipped says why a candidate reviewer was dropped (printed, not an error
	// while another reviewer remains).
	skipped string
}

// attester produces the drafts a subject is approved with. There are three:
// the reviewer's own assertion, the approving reviews of a pull request, and a
// signature.
type attester interface {
	// label names who approves, for the confirmation prompt.
	label() string
	drafts(ctx context.Context, s approval.Subject, in draftInput) ([]approvalDraft, error)
}

// draftInput is what an attestation commits to besides the subject.
type draftInput struct {
	at       time.Time
	expires  string
	accepted []string
}

type assertedAttester struct{ reviewer string }

func (a assertedAttester) label() string { return a.reviewer }

func (a assertedAttester) drafts(_ context.Context, _ approval.Subject, _ draftInput) ([]approvalDraft, error) {
	return []approvalDraft{{reviewer: a.reviewer, assurance: lockfile.AssuranceAsserted}}, nil
}

// signedAttester signs each approval as a DSSE attestation.
type signedAttester struct {
	signer signing.Signer
	meta   signing.LockMeta
}

func (a signedAttester) label() string { return "the holder of the signing identity" }

func (a signedAttester) drafts(ctx context.Context, s approval.Subject, in draftInput) ([]approvalDraft, error) {
	st, err := signing.ApprovalStatement(signing.ApprovalSubject{Kind: s.Kind, Domain: s.Domain, ID: s.ID, Digest: s.Digest}, signing.ApprovalPredicate{
		AcceptedFindings: in.accepted, Expires: in.expires, ApprovedAt: in.at.Format(time.RFC3339),
		Repository: a.meta.Repository, AIRulezVersion: a.meta.Version,
	})
	if err != nil {
		return nil, oops.Wrap(err)
	}
	bundle, err := signing.SignStatement(ctx, a.signer, st)
	if err != nil {
		return nil, oops.Wrap(err)
	}
	info, err := signing.Inspect(bundle)
	if err != nil {
		return nil, oops.Wrap(err)
	}
	return []approvalDraft{{
		reviewer: approval.NormalizeReviewer(signing.ApprovalReviewer(info)), assurance: lockfile.AssuranceSigned,
		attestation: approval.BundleDigest(bundle), bundle: bundle,
	}}, nil
}

// reviewAttester links the approval to the approving reviews of a pull request.
type reviewAttester struct {
	client forge.Client
	repo   forge.Repo
	pr     int
	git    *approveGit
	// only, when set, limits the result to this reviewer.
	only string
	// env supplies the policy that names reviewers who are not maintainers.
	env *approveEnv
}

func (a reviewAttester) label() string {
	return fmt.Sprintf("the approving reviewers of pull request #%d", a.pr)
}

func (a reviewAttester) drafts(ctx context.Context, s approval.Subject, _ draftInput) ([]approvalDraft, error) {
	reviews, err := approval.ApprovingReviews(ctx, a.client, approval.ReviewQuery{Repo: a.repo, PR: a.pr, Digest: s.Digest, PinnedAt: a.git.pinnedAt(s), Named: a.env.namedFor(s)})
	if err != nil {
		return nil, oops.Wrap(err)
	}
	var out []approvalDraft
	for _, r := range reviews {
		if a.only != "" && !approval.SameReviewer(r.Reviewer(), a.only) {
			continue
		}
		d := approvalDraft{reviewer: approval.NormalizeReviewer(r.Reviewer()), assurance: lockfile.AssuranceReviewLinked, ref: r.URL}
		if r.Author {
			d.skipped = "is the author of the pull request"
		}
		out = append(out, d)
	}
	if len(out) == 0 {
		return nil, oops.Hint("the review must be an APPROVED one, not withdrawn, made on a commit of the pull request whose lock pins this digest").
			Errorf("pull request #%d has no approving review of %s at %s", a.pr, safeText(s.Ref()), shortDigest(s.Digest))
	}
	return out, nil
}

// approveForge builds the forge client of the online checks. Tests replace it.
var approveForge = func() forge.Client { return forge.NewClient(forge.Options{}) }

// originRepo is the forge repository of the project's git origin.
func originRepo(ctx context.Context, e *approveEnv) (forge.Repo, error) {
	remote, _ := detectRepo(ctx, e.cfg.BaseDir, ambient.Env(nil))
	if remote == "" {
		return forge.Repo{}, oops.Errorf("cannot tell which repository this is: set an origin remote that points at the forge")
	}
	repo, err := forge.ParseRepo(remote)
	if err != nil {
		return forge.Repo{}, oops.Wrap(err)
	}
	return repo, nil
}

// newAttester picks how the approval is made from the flags.
func (e *approveEnv) newAttester(ctx context.Context) (attester, error) {
	switch {
	case approveSign:
		signer, err := newSigner(ctx, nil)
		if err != nil {
			return nil, err
		}
		meta := signing.LockMeta{Version: Version, Now: e.now}
		meta.Repository, meta.Ref = detectRepo(ctx, e.cfg.BaseDir, nil)
		return signedAttester{signer: signer, meta: meta}, nil
	case approveFromReview > 0:
		return e.reviewAttester(ctx)
	}
	reviewer, err := e.reviewer()
	if err != nil {
		return nil, err
	}
	return assertedAttester{reviewer: reviewer}, nil
}

func (e *approveEnv) reviewAttester(ctx context.Context) (attester, error) {
	repo, err := originRepo(ctx, e)
	if err != nil {
		return nil, err
	}
	g, err := newApproveGit(ctx, e.cfg)
	if err != nil {
		return nil, err
	}
	return reviewAttester{client: approveForge(), repo: repo, pr: approveFromReview, git: g, only: approveReviewer, env: e}, nil
}

// namedFor returns who the policy names for s ([governance] approvers or
// CODEOWNERS): a reviewer who is not an owner, member or collaborator of the
// repository counts only when named, since anyone can review a public repository.
func (e *approveEnv) namedFor(s approval.Subject) func(login string) bool {
	return func(login string) bool { return e.policy.Names("github:"+login, s) }
}

// refuseDenied stops the approval of a digest on the deny list.
func (e *approveEnv) refuseDenied(subs []approval.Subject) error {
	if err := e.refuseOrgDenied(subs); err != nil {
		return err
	}
	for _, s := range subs {
		if reason, denied := e.policy.Deny[s.Digest]; denied {
			return oops.Hint("remove or replace the content").Errorf("%s %s is on the deny list%s and cannot be approved",
				approval.CodeDenied, safeText(s.Ref()), reasonSuffix(safeText(reason)))
		}
	}
	return nil
}

// refuseOrgDenied refuses a digest the organization policy denies
// (sources.deny_digests, AR747): generation refuses that content, so an approval
// of it would change nothing. A policy that cannot be loaded is not a reason to
// refuse here; the commands that load the policy report it.
func (e *approveEnv) refuseOrgDenied(subs []approval.Subject) error {
	resolved, err := policyEnforcer.LoadFor(e.cfg.BaseDir)
	if err != nil || resolved == nil || len(resolved.Policy.Sources.DenyDigests) == 0 {
		return nil //nolint:nilerr // see above
	}
	for _, s := range subs {
		if slices.Contains(resolved.Policy.Sources.DenyDigests, s.Digest) {
			return oops.Hint("remove or replace the content").Errorf("%s %s is denied by the organization policy (sources.deny_digests) and cannot be approved",
				lint.CodeDigestDenied, safeText(s.Ref()))
		}
	}
	return nil
}

// resolveTeams reads team members from the forge (--resolve-teams) for the
// teams the authorization of subs depends on. A team that cannot be read fails
// the command: authorization never guesses.
func (e *approveEnv) resolveTeams(ctx context.Context, subs []approval.Subject) error {
	if !approveResolveTeams {
		return nil
	}
	entries := append([]string(nil), e.policy.Approvers...)
	if e.policy.Owners != nil {
		for _, s := range subs {
			owners, _ := e.policy.Owners.OwnersOf(s)
			entries = append(entries, owners...)
		}
	}
	teams := e.policy.Teams.Unresolved(entries)
	if len(teams) == 0 {
		return nil
	}
	repo, err := originRepo(ctx, e)
	if err != nil {
		return err
	}
	resolved, err := approval.ResolveTeams(ctx, approveForge(), repo.Host, teams)
	if err != nil {
		return oops.Hint("--resolve-teams needs a token that can read the organization (read:org); or list the members in [governance.teams]").Wrap(err)
	}
	e.policy = e.policy.WithResolvedTeams(resolved)
	return nil
}

// authorize decides whether reviewer may approve s: the allowlist, and with
// approvers_from the CODEOWNERS owners of its path. The error says which.
func (e *approveEnv) authorize(reviewer string, s approval.Subject) error {
	p := e.policy
	if !p.Authorized(reviewer) {
		return oops.Hint("the allowed reviewers are set in [governance] approvers").
			Errorf("%s is not in [governance] approvers", safeText(reviewer))
	}
	if p.Owners == nil {
		return nil
	}
	if problem := p.OwnersProblem(); problem != "" {
		return oops.Errorf("%s %s", approval.CodeUnresolved, safeText(problem))
	}
	owners, covered := p.Owners.OwnersOf(s)
	if !covered {
		return oops.Hint("add a CODEOWNERS line for it").Errorf("no CODEOWNERS entry owns %s, so nobody may approve it", safeText(p.Owners.PathOf(s)))
	}
	if !p.Teams.Matches(owners, reviewer) {
		hint := ""
		if unresolved := p.Teams.Unresolved(owners); len(unresolved) > 0 {
			hint = "; the members of " + strings.Join(unresolved, ", ") + " are unknown (use --resolve-teams or [governance.teams])"
		}
		return oops.Errorf("%s is not a code owner of %s (owners: %s)%s", safeText(reviewer), safeText(p.Owners.PathOf(s)), safeText(strings.Join(owners, ", ")), hint)
	}
	return nil
}

// selfAuthors caches the authors of changes to each subject, for
// forbid_self_approval.
type selfAuthors struct {
	git  *approveGit
	base string
	seen map[string][]string
}

func (e *approveEnv) newSelfAuthors(ctx context.Context) (*selfAuthors, error) {
	if !e.policy.ForbidSelf {
		return nil, nil //nolint:nilnil // nil means the check is off
	}
	g, err := newApproveGit(ctx, e.cfg)
	if err != nil {
		return nil, err
	}
	base, err := g.baseRevision(approveBase)
	if err != nil {
		return nil, err
	}
	return &selfAuthors{git: g, base: base, seen: map[string][]string{}}, nil
}

// check reports an error when reviewer authored a change to s since the base revision.
func (a *selfAuthors) check(reviewer string, s approval.Subject) error {
	if a == nil {
		return nil
	}
	emails, ok := a.seen[s.Key()]
	if !ok {
		var err error
		if emails, err = a.git.authors(a.base, a.git.subjectPaths(s)); err != nil {
			return err
		}
		a.seen[s.Key()] = emails
	}
	for _, email := range emails {
		if approval.AuthorIs(reviewer, email) {
			return oops.Hint("another reviewer must approve it").
				Errorf("%s authored a change to %s since %s and [governance] forbid_self_approval is set", safeText(reviewer), safeText(s.Ref()), safeText(a.base))
		}
	}
	return nil
}

// settle applies authorization and forbid_self_approval to the drafts of one
// subject. A review-linked candidate that fails is dropped with a note while
// another remains; any other failure is an error.
func (e *approveEnv) settle(s approval.Subject, drafts []approvalDraft, self *selfAuthors, out func(string)) ([]approvalDraft, error) {
	var keep []approvalDraft
	var firstErr error
	for _, d := range drafts {
		err := e.authorize(d.reviewer, s)
		if err == nil && d.skipped != "" && e.policy.ForbidSelf {
			err = oops.Errorf("%s %s", safeText(d.reviewer), d.skipped)
		}
		if err == nil {
			err = self.check(d.reviewer, s) //nolint:contextcheck // the history probes of gitutil take no context
		}
		if err != nil {
			if d.assurance != lockfile.AssuranceReviewLinked {
				return nil, err
			}
			if firstErr == nil {
				firstErr = err
			}
			out(fmt.Sprintf("skipped %s: %v", safeText(d.reviewer), err))
			continue
		}
		keep = append(keep, d)
	}
	if len(keep) == 0 {
		if firstErr == nil {
			firstErr = errors.New("no reviewer remains")
		}
		return nil, oops.Hint("another reviewer who is allowed to approve it must review the pull request").
			Errorf("no approving review of %s qualifies: %v", safeText(s.Ref()), firstErr)
	}
	return keep, nil
}

// writeBundles stores the attestation bundles next to the lock, under
// attestations/, named by their digest.
func (e *approveEnv) writeBundles(drafts []approvalDraft) error {
	for _, d := range drafts {
		if len(d.bundle) == 0 {
			continue
		}
		path, err := approval.AttestationFile(e.cfg.ConfigDir, d.attestation)
		if err != nil {
			return oops.Wrap(err)
		}
		if err := writeBundle(path, d.bundle); err != nil {
			return err
		}
	}
	return nil
}

// validateFormatAndAssurance checks --format and the flags of the assurance modes.
func validateFormatAndAssurance() error {
	if err := checkFormatFlag(approveFormat); err != nil {
		return err
	}
	for _, check := range []func() error{validateRecordModes, validateSignFlagSet, validateRevokeDenyFlags, validateOnlineFlags} {
		if err := check(); err != nil {
			return err
		}
	}
	return nil
}

// recordsApproval reports a run that writes approvals (not --list, --revoke, --diff, --prune or --verify-base).
func recordsApproval() bool {
	return !approveList && !approveRevoke && !approveDiff && !approvePrune && approveVerifyBase == ""
}

func validateRecordModes() error {
	switch {
	case approveSign && approveFromReview > 0:
		return oops.Errorf("--sign and --from-github-review are separate ways to approve: pick one")
	case (approveSign || approveFromReview > 0) && !recordsApproval():
		return oops.Errorf("--sign and --from-github-review record approvals: they do not combine with --list, --revoke, --diff, --prune or --verify-base")
	case approveFromReview < 0:
		return oops.Errorf("--from-github-review takes a pull request number")
	case approveSign && approveReviewer != "":
		return oops.Errorf("--reviewer does not apply to --sign: the signer's identity is the reviewer")
	}
	return nil
}

func anyOf(conds ...bool) bool {
	for _, c := range conds {
		if c {
			return true
		}
	}
	return false
}

func validateSignFlagSet() error {
	signFlag := anyOf(signKey != "", signKeyless, signKeyPassEnv != "", signTokenEnv != "", signInteractive, signFulcioURL != "", signRekorURL != "", signTLog)
	if !approveSign {
		if signFlag {
			return oops.Errorf("--key, --keyless, --identity-token-env, --interactive, --fulcio-url, --rekor-url, --tlog and --key-password-env apply to --sign")
		}
		return nil
	}
	if signKey == "" && !signKeyless {
		return oops.Hint("pass --key <file> for key mode, or --keyless").Errorf("choose how to sign")
	}
	for _, rule := range []struct {
		bad bool
		msg string
	}{
		{signKey != "" && signKeyless, "--key and --keyless are mutually exclusive"},
		{signKeyless && signTLog, "--tlog applies to --key: keyless signatures are always logged"},
		{!signKeyless && anyOf(signTokenEnv != "", signFulcioURL != "", signInteractive), "--identity-token-env, --interactive and --fulcio-url apply to --keyless"},
		{signKeyless && signKeyPassEnv != "", "--key-password-env applies to --key"},
		{signRekorURL != "" && !signKeyless && !signTLog, "--rekor-url applies to --keyless or --tlog: without a log the signature is never sent anywhere"},
	} {
		if rule.bad {
			return oops.New(rule.msg) //nolint:wrapcheck // a plain message
		}
	}
	if err := checkSigstoreURL("--fulcio-url", signFulcioURL); err != nil {
		return err
	}
	return checkSigstoreURL("--rekor-url", signRekorURL)
}

func validateRevokeDenyFlags() error {
	switch {
	case approveDeny && !approveRevoke:
		return oops.Errorf("--deny applies to --revoke")
	case approveReason != "" && !approveDeny:
		return oops.Errorf("--reason applies to --deny")
	case len(approveReason) > 500 || strings.ContainsFunc(approveReason, func(r rune) bool { return r < 0x20 && r != '\t' || r == 0x7f }):
		return oops.Errorf("invalid --reason: at most 500 characters on one line")
	}
	return nil
}

func validateOnlineFlags() error {
	switch {
	case approveResolveTeams && (approveRevoke || approveDiff || approvePrune || approveVerifyBase != ""):
		return oops.Errorf("--resolve-teams applies to --list and to approving")
	case approveBase != "" && (!recordsApproval() || strings.HasPrefix(strings.TrimSpace(approveBase), "-")):
		return oops.Errorf("--base applies to approving and takes a revision")
	}
	return nil
}
