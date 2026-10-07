package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"regexp"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/samber/oops"
	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/approval"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/govview"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/signing"
)

var (
	approveList     bool
	approveAll      bool
	approveRevoke   bool
	approveDiff     bool
	approvePrune    bool
	approveYes      bool
	approveAccept   []string
	approveReviewer string
	approveNote     string
	approveExpires  string
	approveAt       string
	approveFormat   string
	// approveVerifyBase is --verify-base: the git revision approvals are compared with.
	approveVerifyBase string
	// approveBase is --base: the revision forbid_self_approval counts authors from.
	approveBase string
	// approveFromReview is --from-github-review: the pull request whose approving reviews are linked.
	approveFromReview   int
	approveResolveTeams bool
	approveSign         bool
	approveDeny         bool
	approveReason       string
)

// ApproveCmd records, lists and revokes reviewer approvals in ai-rulez.lock.
var ApproveCmd = &cobra.Command{
	Use:   "approve [item...]",
	Short: "Record that you reviewed content, bound to its digest, in ai-rulez.lock",
	Long: `Record an approval in ai-rulez.lock: reviewer R read the content whose digest is D
and accepted it. The record is bound to the digest, so it stops applying the
moment the content changes (a CRLF-only edit does not change the digest; a
flipped executable bit does). [governance] require_approval chooses what needs
one; validate --strict, lock --check, generate --locked and the skills server
enforce it. See docs/approvals.md.

An item is named kind:id or kind:domain/id (skill:backend/deploy, hook:PreToolUse:*:0,
include:shared, installed-skill:name, source:name, served:name, settings:mcp-servers);
a bare id works when it is unambiguous. Remote content must be pinned first
(ai-rulez lock), and approving never fetches.

  ai-rulez approve --list                  status of everything that needs approval
  ai-rulez approve --diff include:shared   what you would be approving
  ai-rulez approve include:shared --reviewer alice --note "read run.sh" --yes
  ai-rulez approve include:shared --accept AR005   accept a finding you read
  ai-rulez approve --revoke include:shared
  ai-rulez approve --prune                 drop stale and orphaned records
  ai-rulez approve --verify-base origin/main   CI: approvals added with the content they approve
  ai-rulez approve include:shared --from-github-review 42 --yes   link the approving reviews of a pull request
  ai-rulez approve include:shared --sign --key approver.key --yes   record a signed approval (DSSE)
  ai-rulez approve --revoke include:shared --deny --reason "exfiltrates ~/.ssh"   revoke and deny the digest

approve prints the files and the security scan findings first. It refuses
content with an error-level finding unless you name its code with --accept (the
codes are stored with the record), and without --yes it asks on a terminal and
refuses elsewhere, so a script cannot approve by accident. The reviewer
defaults to $AI_RULEZ_REVIEWER, else the git user.email. An approval is a
human assertion backed by review of the lock change, not a safety proof.

An approval in the committed lock is an assertion, not authentication: anyone who
can edit the lock can add one. --verify-base <rev> is the CI control for that: it
compares the lock with the one at the merge base of <rev> and HEAD and reports
(AR716) every approval added since for content that was added or changed in the
same range. See docs/approvals.md.

--from-github-review <pr> records one review-linked approval per approving review
of the pull request (reviewer github:<login>, ref the review URL), after checking
through the forge API that the review is APPROVED, not withdrawn, and made on the
commit the content is at (or the pull request's final head when nothing under the
configuration directory changed since). --sign signs the approval as an in-toto
statement (DSSE) with --key or --keyless; the signer's identity becomes the
reviewer and [[signing.trust]] entries with subject = "approval" say who may sign.
--resolve-teams reads @org/team members from the forge for [governance]
approvers and approvers_from. Verify them later with
"ai-rulez verify --approvals [--online]".

Exit codes: 0 ok; 1 the command could not run or refused; 2 --verify-base found
an approval added together with its content.`,
	Args: cobra.ArbitraryArgs,
	Run:  runApprove,
}

func init() {
	f := ApproveCmd.Flags()
	f.BoolVar(&approveList, "list", false, "List what needs approval and its status")
	f.BoolVar(&approveAll, "all", false, "With --list: also list pinned content that needs no approval")
	f.BoolVar(&approveRevoke, "revoke", false, "Remove the approvals of the named items (with --reviewer: only that reviewer's)")
	f.BoolVar(&approveDiff, "diff", false, "Show the files, scan findings and previous approval of the named items; writes nothing")
	f.StringVar(&approveVerifyBase, "verify-base", "", "Report approvals added since this git revision for content that also changed since it (AR716); exit 2 when found; writes nothing")
	f.IntVar(&approveFromReview, "from-github-review", 0, "Record review-linked approvals from the approving reviews of this pull request number (needs the network and a token)")
	f.BoolVar(&approveResolveTeams, "resolve-teams", false, "Expand @org/team entries of approvers and CODEOWNERS from the forge (needs a token with read:org)")
	f.BoolVar(&approveSign, "sign", false, "Sign the approval (DSSE attestation) with --key or --keyless; the signer's identity becomes the reviewer")
	f.StringVar(&signKey, "key", "", "With --sign: PEM private key to sign with (ECDSA or ed25519; cosign keys work)")
	f.StringVar(&signKeyPassEnv, "key-password-env", "", "With --sign --key: environment variable holding the key password (default AI_RULEZ_SIGNING_KEY_PASSWORD, then COSIGN_PASSWORD)")
	f.BoolVar(&signKeyless, "keyless", false, "With --sign: Fulcio certificate and Rekor log entry (network; the log is public)")
	f.StringVar(&signTokenEnv, "identity-token-env", "", "With --sign --keyless: environment variable holding the OIDC token (default: the GitHub Actions runtime token)")
	f.BoolVar(&signInteractive, "interactive", false, "With --sign --keyless: open a browser for the OIDC login when no token is available")
	f.StringVar(&signFulcioURL, "fulcio-url", "", "With --sign --keyless: Fulcio URL (default "+signing.DefaultFulcioURL+")")
	f.StringVar(&signRekorURL, "rekor-url", "", "With --sign: Rekor URL for --keyless or --tlog (default "+signing.DefaultRekorURL+")")
	f.BoolVar(&signTLog, "tlog", false, "With --sign --key: also record the signature in the Rekor transparency log (network; public log)")
	f.BoolVar(&approveDeny, "deny", false, "With --revoke: also add the digest of the item to the deny list (AR717)")
	f.StringVar(&approveReason, "reason", "", "With --deny: why the digest is denied (stored in the lock; scanned for secrets)")
	f.StringVar(&approveBase, "base", "", "With [governance] forbid_self_approval: count authors of changes since this revision (default: the branch's upstream)")
	f.BoolVar(&approvePrune, "prune", false, "Remove approvals of content that no longer exists or whose digest changed")
	f.BoolVar(&approveYes, "yes", false, "Do not ask for confirmation (required without a terminal)")
	f.StringSliceVar(&approveAccept, "accept", nil, "Accept this scan finding code (repeatable); stored with the approval")
	f.StringVar(&approveReviewer, "reviewer", "", "Reviewer to record (default: $AI_RULEZ_REVIEWER, else git user.email)")
	f.StringVar(&approveNote, "note", "", "Free-text note stored with the approval (scanned for secrets)")
	f.StringVar(&approveExpires, "expires", "", "Expiry date YYYY-MM-DD (default: today + [governance] max_age, else none)")
	f.StringVar(&approveAt, "at", "", "Approval time (RFC 3339 or YYYY-MM-DD) for reproducible runs (default: SOURCE_DATE_EPOCH, else now)")
	addFormatFlag(f, &approveFormat, "", formatText, formatText, formatJSON) // of --list
	addJSONFlagAlias(f)
	f.StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
}

// approveReviewerPattern bounds what is written to the committed lock as a reviewer.
var approveReviewerPattern = regexp.MustCompile(`^[^\x00-\x1f\x7f]{1,200}$`)

var approveCodePattern = regexp.MustCompile(`^AR[0-9A-Z]{3,4}$`)

func runApprove(_ *cobra.Command, args []string) {
	if err := validateApproveFlags(args); err != nil {
		fmtError(err)
		os.Exit(1)
	}
	if code := approveRun(os.Stdout, args); code != 0 {
		os.Exit(code)
	}
}

func validateApproveFlags(args []string) error {
	if err := validateFormatAndAssurance(); err != nil {
		return err
	}
	if err := validateApproveModes(args); err != nil {
		return err
	}
	return validateApproveInputs()
}

// validateApproveModes checks that one mode is chosen and its flags fit it.
func validateApproveModes(args []string) error {
	modes := 0
	for _, on := range []bool{approveList, approveRevoke, approveDiff, approvePrune, approveVerifyBase != ""} {
		if on {
			modes++
		}
	}
	switch {
	case modes > 1:
		return oops.Errorf("--list, --revoke, --diff, --prune and --verify-base are mutually exclusive")
	case approveFormat != "" && !approveList:
		return oops.Errorf("--format applies to --list only")
	case approveAll && !approveList:
		return oops.Errorf("--all applies to --list only")
	}
	return validateApproveTargets(args)
}

// validateApproveTargets checks the item names and --verify-base against the mode.
func validateApproveTargets(args []string) error {
	switch {
	case (approveList || approvePrune || approveVerifyBase != "") && len(args) > 0:
		return oops.Errorf("--list, --prune and --verify-base take no item names")
	case approveVerifyBase != "" && strings.HasPrefix(strings.TrimSpace(approveVerifyBase), "-"):
		return oops.Errorf("invalid --verify-base %q: a git revision", approveVerifyBase)
	case !approveList && !approvePrune && approveVerifyBase == "" && len(args) == 0:
		return oops.Hint("see `ai-rulez approve --list` for what needs approval").Errorf("name the item(s) to approve")
	}
	return nil
}

// validateApproveInputs checks the free-text flags: --accept, --reviewer, --note.
func validateApproveInputs() error {
	for _, code := range approveAccept {
		if !approveCodePattern.MatchString(strings.ToUpper(code)) {
			return oops.Errorf("invalid --accept %q: expected a rule code such as AR005", code)
		}
	}
	if approveReviewer != "" && !approveReviewerPattern.MatchString(approveReviewer) {
		return oops.Errorf("invalid --reviewer: use a single line of at most 200 characters")
	}
	if len(approveNote) > 500 || strings.ContainsFunc(approveNote, isNoteControl) {
		return oops.Errorf("invalid --note: at most 500 characters, no control characters")
	}
	return nil
}

// isNoteControl reports a control character a note may not carry (newline and
// tab are allowed).
func isNoteControl(r rune) bool { return r < 0x20 && r != '\n' && r != '\t' || r == 0x7f }

// approveEnv is what every mode of the command works on.
type approveEnv struct {
	cfg      *config.Config
	lock     *lockfile.File
	policy   approval.Policy
	items    []lockfile.Item
	subjects []approval.Subject
	now      time.Time
}

func loadApproveEnv() (*approveEnv, error) { return loadApproveEnvAt("") }

func loadApproveEnvAt(path string) (*approveEnv, error) {
	cfg, _, err := loadForLockCheck(path)
	if err != nil {
		return nil, err
	}
	lock, err := lockfile.Load(cfg.ConfigDir)
	if err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	if lock == nil {
		return nil, oops.Hint("run `ai-rulez lock` first: only pinned content can be approved").Errorf("no %s in %s", lockfile.FileName, cfg.ConfigDir)
	}
	snap, err := lockSnapshot(cmdContext(), cfg, lock.Profile, true)
	if err != nil {
		return nil, err
	}
	env := &approveEnv{cfg: cfg, lock: lock, policy: approval.PolicyOf(cfg), items: snap.Items, now: govview.ApprovalNow()}
	env.subjects = approval.SubjectsOf(lock, snap.Items)
	return env, nil
}

func approveRun(out io.Writer, args []string) int {
	env, err := loadApproveEnv()
	if err != nil {
		fmtError(err)
		return 1
	}
	switch {
	case approveVerifyBase != "":
		var code int
		code, err = env.verifyBase(out, approveVerifyBase)
		if err != nil {
			fmtError(err)
		}
		return code
	case approveList:
		err = env.list(out)
	case approvePrune:
		err = env.prune(out)
	case approveRevoke:
		err = env.revoke(out, args)
	case approveDiff:
		err = env.diff(out, args)
	default:
		err = env.approve(out, args)
	}
	if err != nil {
		fmtError(err)
		return 1
	}
	return 0
}

// ---- --list ----

// ApproveListSchemaVersion versions the JSON of `approve --list --format json`.
const ApproveListSchemaVersion = 1

type approveListDoc struct {
	SchemaVersion int                 `json:"schema_version"`
	Policy        approveListPolicy   `json:"policy"`
	Items         []approveListItem   `json:"items"`
	Orphans       []approveListOrphan `json:"orphans"`
	Summary       map[string]int      `json:"summary"`
}

type approveListPolicy struct {
	RequireApproval []string `json:"require_approval"`
	Exempt          []string `json:"exempt"`
	MinApprovers    int      `json:"min_approvers"`
	Approvers       []string `json:"approvers"`
	Enforce         bool     `json:"enforce"`
	MinAssurance    string   `json:"min_assurance,omitempty"`
	ApproversFrom   string   `json:"approvers_from,omitempty"`
	ForbidSelf      bool     `json:"forbid_self_approval,omitempty"`
}

type approveListItem struct {
	Ref            string   `json:"ref"`
	Kind           string   `json:"kind"`
	ID             string   `json:"id"`
	Domain         string   `json:"domain,omitempty"`
	Digest         string   `json:"digest"`
	Required       bool     `json:"required"`
	Status         string   `json:"status"`
	Code           string   `json:"code,omitempty"`
	Reviewers      []string `json:"reviewers"`
	Assurance      string   `json:"assurance,omitempty"`
	Expires        string   `json:"expires,omitempty"`
	ApprovedDigest string   `json:"approved_digest,omitempty"`
	Detail         string   `json:"detail,omitempty"`
}

type approveListOrphan struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Domain   string `json:"domain,omitempty"`
	Digest   string `json:"digest"`
	Reviewer string `json:"reviewer"`
}

func emptyIfNil(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

func (e *approveEnv) listDoc() *approveListDoc {
	p := e.policy
	doc := &approveListDoc{
		SchemaVersion: ApproveListSchemaVersion,
		Policy: approveListPolicy{RequireApproval: emptyIfNil(p.Selectors), Exempt: emptyIfNil(p.Exempt), MinApprovers: max(p.MinApprovers, 1),
			Approvers: emptyIfNil(p.Approvers), Enforce: p.Enforce, MinAssurance: p.MinAssurance, ApproversFrom: e.approversFrom(), ForbidSelf: p.ForbidSelf},
		Items: []approveListItem{}, Orphans: []approveListOrphan{}, Summary: map[string]int{},
	}
	hasRecord := map[string]bool{}
	for i := range e.lock.Approval {
		a := &e.lock.Approval[i]
		hasRecord[a.ItemKey()] = true
	}
	results := e.policy.EvaluateAll(e.lock.Approval, e.subjects, e.now)
	for i := range results {
		r := &results[i]
		if !r.Required && !approveAll && !hasRecord[r.Key()] {
			continue
		}
		who := r.Reviewers
		if len(who) == 0 {
			who = r.Recorded // an expired or unauthorized row still says who approved it
		}
		doc.Items = append(doc.Items, approveListItem{
			Ref: r.Ref(), Kind: r.Kind, ID: r.ID, Domain: r.Domain, Digest: r.Digest, Required: r.Required, Status: r.Status,
			Code: approval.CodeOf(r.Status), Reviewers: emptyIfNil(who), Assurance: r.Assurance, Expires: r.Expires, ApprovedDigest: r.ApprovedDigest,
			Detail: safeText(r.Detail),
		})
		if r.Required {
			doc.Summary["required"]++
			doc.Summary[r.Status]++
		}
	}
	orphans := approval.Orphans(e.lock.Approval, e.subjects)
	for i := range orphans {
		a := &orphans[i]
		doc.Orphans = append(doc.Orphans, approveListOrphan{Kind: a.Kind, ID: a.ID, Domain: a.Domain, Digest: a.Digest, Reviewer: a.Reviewer})
	}
	return doc
}

func shortDigest(d string) string {
	hex := strings.TrimPrefix(d, "sha256:")
	if len(hex) > 12 {
		return hex[:12] + "…"
	}
	return hex
}

func (e *approveEnv) approversFrom() string {
	if e.cfg.Governance == nil {
		return ""
	}
	return e.cfg.Governance.ApproversFrom
}

// approveListRow is the tab-separated text row of one item of `approve --list`.
func approveListRow(it *approveListItem) string {
	reviewer, expires := "-", "-"
	if len(it.Reviewers) > 0 {
		reviewer = safeText(strings.Join(it.Reviewers, ","))
	}
	if it.Expires != "" {
		expires = it.Expires
	}
	status := it.Status
	if !it.Required && status == approval.StatusNotRequired {
		status = "-"
	}
	id := it.ID
	if it.Domain != "" {
		id = it.Domain + "/" + id
	}
	assurance := "-"
	if it.Assurance != "" {
		assurance = it.Assurance
	}
	return fmt.Sprintf("%s\t%s\t%s\t%s\t%s\t%s\t%s\n", it.Kind, safeText(id), shortDigest(it.Digest), status, reviewer, expires, assurance)
}

func (e *approveEnv) list(out io.Writer) error {
	if err := e.resolveTeams(cmdContext(), e.subjects); err != nil {
		return err
	}
	doc := e.listDoc()
	if approveFormat == formatJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(doc)
	}
	if len(doc.Items) == 0 && len(doc.Orphans) == 0 {
		if !e.policy.Active() {
			_, err := fmt.Fprintln(out, "nothing needs approval: [governance] require_approval is not set (see docs/approvals.md)")
			return err
		}
		_, err := fmt.Fprintln(out, "no pinned content needs approval")
		return err
	}
	tw := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "KIND\tID\tDIGEST\tSTATUS\tREVIEWER\tEXPIRES\tASSURANCE") //nolint:errcheck // flushed below
	for i := range doc.Items {
		fmt.Fprint(tw, approveListRow(&doc.Items[i])) //nolint:errcheck // flushed below
	}
	for _, o := range doc.Orphans {
		id := o.ID
		if o.Domain != "" {
			id = o.Domain + "/" + id
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\torphan\t%s\t-\t-\n", o.Kind, safeText(id), shortDigest(o.Digest), safeText(o.Reviewer)) //nolint:errcheck // flushed below
	}
	if err := tw.Flush(); err != nil {
		return oops.Wrapf(err, "write the list")
	}
	s := doc.Summary
	_, err := fmt.Fprintf(out, "%d require approval: %d ok, %d stale, %d missing, %d expired, %d unauthorized, %d insufficient",
		s["required"], s[approval.StatusOK], s[approval.StatusStale], s[approval.StatusMissing], s[approval.StatusExpired],
		s[approval.StatusUnauthorized], s[approval.StatusInsufficient])
	if err != nil {
		return err //nolint:wrapcheck // write error
	}
	if s[approval.StatusDenied] > 0 || s[approval.StatusUnverified] > 0 {
		_, err = fmt.Fprintf(out, ", %d denied, %d unverified", s[approval.StatusDenied], s[approval.StatusUnverified])
		if err != nil {
			return err //nolint:wrapcheck // write error
		}
	}
	_, err = fmt.Fprintln(out)
	return err
}

// ---- --revoke and --prune ----

func (e *approveEnv) save() error {
	return lockfile.Save(e.cfg.ConfigDir, e.lock)
}

func (e *approveEnv) revoke(out io.Writer, refs []string) error {
	if err := e.checkText("--reason", approveReason); err != nil {
		return err
	}
	var dropped []lockfile.Approval
	// Orphaned records can be revoked by name too: they name content that is gone.
	known := append([]approval.Subject(nil), e.subjects...)
	orphans := approval.Orphans(e.lock.Approval, e.subjects)
	for i := range orphans {
		a := &orphans[i]
		known = append(known, approval.Subject{Kind: a.Kind, Domain: a.Domain, ID: a.ID, Digest: a.Digest})
	}
	removed := 0
	for _, ref := range refs {
		s, err := approval.Resolve(known, ref)
		if err != nil {
			return oops.Wrap(err)
		}
		kept := e.lock.Approval[:0:0]
		n := 0
		for i := range e.lock.Approval {
			a := &e.lock.Approval[i]
			if a.ItemKey() == s.Key() && (approveReviewer == "" || approval.SameReviewer(a.Reviewer, approveReviewer)) {
				n++
				dropped = append(dropped, *a)
				continue
			}
			kept = append(kept, *a)
		}
		if n == 0 {
			return oops.Errorf("%s has no approval to revoke", s.Ref())
		}
		e.lock.Approval = kept
		removed += n
		if _, err := fmt.Fprintf(out, "revoked %d approval(s) of %s\n", n, safeText(s.Ref())); err != nil {
			return oops.Wrapf(err, "write output")
		}
		if approveDeny {
			e.lock.SetDeny(lockfile.Deny{Digest: s.Digest, Reason: approveReason})
			if _, err := fmt.Fprintf(out, "denied %s %s\n", safeText(s.Ref()), s.Digest); err != nil {
				return oops.Wrapf(err, "write output")
			}
		}
	}
	if removed == 0 {
		return nil
	}
	if err := e.save(); err != nil {
		return err
	}
	e.removeUnusedBundles(dropped)
	return nil
}

// removeUnusedBundles deletes the attestation bundles of removed approvals that
// no remaining record names, so a revoked or pruned approval leaves no signature
// of it behind. Only files under the attestations directory are touched, and a
// failure to delete is a warning: the lock is already written.
func (e *approveEnv) removeUnusedBundles(dropped []lockfile.Approval) {
	inUse := map[string]bool{}
	for i := range e.lock.Approval {
		a := &e.lock.Approval[i]
		inUse[a.Attestation] = true
	}
	for i := range dropped {
		a := &dropped[i]
		if a.Attestation == "" || inUse[a.Attestation] {
			continue
		}
		inUse[a.Attestation] = true
		path, err := approval.AttestationFile(e.cfg.ConfigDir, a.Attestation)
		if err != nil {
			continue
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			logger.Warn("Could not remove the attestation bundle of a removed approval", "path", path, "error", err)
		}
	}
}

func (e *approveEnv) prune(out io.Writer) error {
	current := map[string]string{}
	for _, s := range e.subjects {
		current[s.Key()] = s.Digest
	}
	kept := e.lock.Approval[:0:0]
	var dropped []lockfile.Approval
	for i := range e.lock.Approval {
		a := &e.lock.Approval[i]
		if d, ok := current[a.ItemKey()]; ok && d == a.Digest {
			kept = append(kept, *a)
		} else {
			dropped = append(dropped, *a)
		}
	}
	n := len(e.lock.Approval) - len(kept)
	if n == 0 {
		_, err := fmt.Fprintln(out, "nothing to prune")
		return err
	}
	e.lock.Approval = kept
	if err := e.save(); err != nil {
		return err
	}
	e.removeUnusedBundles(dropped)
	_, err := fmt.Fprintf(out, "pruned %d stale or orphaned approval(s)\n", n)
	return err
}

// ---- --diff and approve ----

// approveReview is what the reviewer is shown for one item.
type approveReview struct {
	subject  approval.Subject
	files    []approvedFile
	note     string
	findings []lint.Finding
	previous []lockfile.Approval
}

func (e *approveEnv) review(s approval.Subject) approveReview {
	rv := approveReview{subject: s}
	rv.files, rv.note = subjectFiles(e.cfg, s)
	rv.findings = scanApproved(e.cfg, s.ID, rv.files)
	for i := range e.lock.Approval {
		a := &e.lock.Approval[i]
		if a.ItemKey() == s.Key() {
			rv.previous = append(rv.previous, *a)
		}
	}
	return rv
}

func (e *approveEnv) printReview(out io.Writer, rv *approveReview) {
	s := rv.subject
	fmt.Fprintf(out, "%s  %s\n", safeText(s.Ref()), s.Digest) //nolint:errcheck // terminal output
	for i := range rv.previous {
		a := &rv.previous[i]
		state := "approved"
		if a.Digest != s.Digest {
			state = "previously approved at " + shortDigest(a.Digest) + " (now changed)"
		}
		fmt.Fprintf(out, "  %s by %s at %s\n", state, safeText(a.Reviewer), a.ApprovedAt) //nolint:errcheck // terminal output
	}
	for _, f := range rv.files {
		extra := ""
		switch {
		case f.Executable:
			extra = " (executable)"
		case f.Data == nil:
			extra = " (too large to scan)"
		}
		fmt.Fprintf(out, "  %s  %d bytes%s\n", safeText(f.Path), f.Size, extra) //nolint:errcheck // terminal output
	}
	if rv.note != "" {
		fmt.Fprintf(out, "  note: %s\n", safeText(rv.note)) //nolint:errcheck // terminal output
	}
	if len(rv.findings) == 0 {
		fmt.Fprintln(out, "  scan: no findings") //nolint:errcheck // terminal output
		return
	}
	fmt.Fprintf(out, "  scan: %d finding(s)\n", len(rv.findings)) //nolint:errcheck // terminal output
	for _, f := range rv.findings {
		fmt.Fprintf(out, "    %s %s %s: %s\n", f.Code, f.Severity, safeText(f.File), safeText(f.Message)) //nolint:errcheck // terminal output
	}
}

func (e *approveEnv) resolveAll(refs []string) ([]approval.Subject, error) {
	var subs []approval.Subject
	for _, ref := range refs {
		s, err := approval.Resolve(e.subjects, ref)
		if err != nil {
			return nil, oops.Wrap(err)
		}
		subs = append(subs, s)
	}
	return subs, nil
}

func (e *approveEnv) diff(out io.Writer, refs []string) error {
	subs, err := e.resolveAll(refs)
	if err != nil {
		return err
	}
	for _, s := range subs {
		rv := e.review(s)
		e.printReview(out, &rv)
	}
	return nil
}

func (e *approveEnv) reviewer() (string, error) {
	r := approveReviewer
	if r == "" {
		r = os.Getenv("AI_RULEZ_REVIEWER")
	}
	if r == "" {
		r = gitUserEmail(e.cfg.BaseDir)
	}
	switch {
	case r == "":
		return "", oops.Hint("pass --reviewer, or set $AI_RULEZ_REVIEWER or git user.email").Errorf("no reviewer: cannot tell who is approving")
	case !approveReviewerPattern.MatchString(r):
		return "", oops.Errorf("the reviewer %q is not a single line of at most 200 characters", safeText(r))
	}
	return approval.NormalizeReviewer(r), nil
}

// approvedAt resolves the time of the record: --at, else the approval clock.
func (e *approveEnv) approvedAt() (time.Time, error) {
	// The stamp may follow SOURCE_DATE_EPOCH (reproducible runs); expiry never does.
	if approveAt == "" {
		return config.ResolveGenerationTime().UTC().Truncate(time.Second), nil
	}
	for _, layout := range []string{time.RFC3339, time.DateOnly} {
		if t, err := time.Parse(layout, approveAt); err == nil {
			if t.After(e.now) {
				return time.Time{}, oops.Errorf("--at %s is in the future: a record stamped ahead of the clock would win over every later approval", approveAt)
			}
			return t.UTC(), nil
		}
	}
	return time.Time{}, oops.Errorf("invalid --at %q: use RFC 3339 or YYYY-MM-DD", approveAt)
}

func (e *approveEnv) expiry(at time.Time) (string, error) {
	switch {
	case approveExpires != "":
		t, err := time.Parse(time.DateOnly, approveExpires)
		if err != nil {
			return "", oops.Errorf("invalid --expires %q: use YYYY-MM-DD", approveExpires)
		}
		if approval.ExpiredAt(approveExpires, e.now) {
			return "", oops.Errorf("--expires %s is in the past", t.Format(time.DateOnly))
		}
		if ceiling, ok := e.policy.Ceiling(at.UTC().Format(time.RFC3339)); ok && approveExpires > ceiling {
			return "", oops.Errorf("--expires %s is later than %s, the end of [governance] max_age; a longer approval needs a longer max_age", approveExpires, ceiling)
		}
		return approveExpires, nil
	case e.policy.MaxAge > 0:
		return at.Add(e.policy.MaxAge).Format(time.DateOnly), nil
	}
	return "", nil
}

func (e *approveEnv) approve(out io.Writer, refs []string) error {
	ctx := cmdContext()
	subs, err := e.resolveAll(refs)
	if err != nil {
		return err
	}
	if err := e.refuseBeforeReview(ctx, subs); err != nil {
		return err
	}
	how, err := e.newAttester(ctx)
	if err != nil {
		return err
	}
	self, err := e.newSelfAuthors(ctx)
	if err != nil {
		return err
	}
	at, err := e.approvedAt()
	if err != nil {
		return err
	}
	expires, err := e.expiry(at)
	if err != nil {
		return err
	}
	if err := e.checkNote(); err != nil {
		return err
	}
	taken, err := e.showAndScan(out, subs)
	if err != nil {
		return err
	}
	if err := e.confirm(out, len(subs), how.label()); err != nil {
		return err
	}
	if err := e.recheck(subs); err != nil {
		return err
	}
	batch, err := e.record(ctx, out, subs, taken, how, self, draftInput{at: at, expires: expires})
	if err != nil {
		return err
	}
	if err := e.writeBundles(batch.drafts); err != nil {
		return err
	}
	for i := range batch.records {
		e.supersede(&batch.records[i])
		e.lock.SetApproval(batch.records[i])
		fmt.Fprintf(out, "approved %s at %s by %s (assurance=%s%s)\n", safeText(batch.subjects[i].Ref()), shortDigest(batch.subjects[i].Digest), //nolint:errcheck // terminal output
			safeText(batch.records[i].Reviewer), batch.records[i].Assurance, expiryText(expires))
	}
	if err := e.save(); err != nil {
		return err
	}
	logger.Success("Updated lock file", "path", lockfile.Path(e.cfg.ConfigDir))
	return nil
}

// refuseBeforeReview stops what no review can fix: a denied digest, role outputs
// whose pin is stale, and teams that cannot be read.
func (e *approveEnv) refuseBeforeReview(ctx context.Context, subs []approval.Subject) error {
	if err := e.refuseDenied(subs); err != nil {
		return err
	}
	if err := e.checkRoleOutputs(ctx, subs); err != nil {
		return err
	}
	return e.resolveTeams(ctx, subs)
}

// showAndScan prints what each item is, scans it, and returns the finding codes
// the reviewer accepted per item. An error-level finding that is not accepted
// refuses the whole approval.
func (e *approveEnv) showAndScan(out io.Writer, subs []approval.Subject) ([][]string, error) {
	accepted := map[string]bool{}
	for _, c := range approveAccept {
		accepted[strings.ToUpper(c)] = true
	}
	taken := make([][]string, len(subs))
	var blocked []string
	for i, s := range subs {
		rv := e.review(s)
		e.printReview(out, &rv)
		for _, f := range rv.findings {
			switch {
			case accepted[f.Code]:
				taken[i] = appendUnique(taken[i], f.Code)
			case f.Severity == lint.SeverityError:
				blocked = appendUnique(blocked, s.Ref()+" "+f.Code)
			}
		}
	}
	if len(blocked) > 0 {
		return nil, oops.Hint("read the findings above; pass --accept <code> only for a finding you reviewed and accept").
			Errorf("refusing to approve content with error-level scan findings: %s", strings.Join(blocked, ", "))
	}
	return taken, nil
}

// approvalBatch is what an approve run is about to write.
type approvalBatch struct {
	subjects []approval.Subject
	drafts   []approvalDraft
	records  []lockfile.Approval
}

// record asks the attester for the approvals of every subject and settles
// authorization and self-approval, without writing anything.
func (e *approveEnv) record(ctx context.Context, out io.Writer, subs []approval.Subject, taken [][]string, how attester, self *selfAuthors, in draftInput) (*approvalBatch, error) {
	batch := &approvalBatch{}
	notice := func(msg string) { fmt.Fprintln(out, msg) } //nolint:errcheck // terminal output
	for i, s := range subs {
		in.accepted = sortedCopy(taken[i])
		drafts, err := how.drafts(ctx, s, in)
		if err != nil {
			return nil, err
		}
		if drafts, err = e.settle(s, drafts, self, notice); err != nil {
			return nil, err
		}
		for _, d := range drafts {
			batch.subjects = append(batch.subjects, s)
			batch.drafts = append(batch.drafts, d)
			batch.records = append(batch.records, lockfile.Approval{
				Kind: s.Kind, ID: s.ID, Domain: s.Domain, Digest: s.Digest, Reviewer: d.reviewer, Assurance: d.assurance,
				ApprovedAt: in.at.Format(time.RFC3339), Expires: in.expires, Note: approveNote, AcceptedFindings: in.accepted,
				Ref: d.ref, Attestation: d.attestation,
			})
		}
	}
	return batch, nil
}

func expiryText(expires string) string {
	if expires == "" {
		return ""
	}
	return ", expires " + expires
}

func appendUnique(in []string, v string) []string {
	for _, x := range in {
		if x == v {
			return in
		}
	}
	return append(in, v)
}

func sortedCopy(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	return slices.Sorted(slices.Values(in))
}

// supersede removes the reviewer's earlier records for the same item at other
// digests: the new record replaces them, so the lock does not collect one stale
// record per version. Other reviewers' records stay until they approve again.
func (e *approveEnv) supersede(rec *lockfile.Approval) {
	kept := e.lock.Approval[:0:0]
	for i := range e.lock.Approval {
		a := &e.lock.Approval[i]
		if a.ItemKey() == rec.ItemKey() && approval.SameReviewer(a.Reviewer, rec.Reviewer) && a.Digest != rec.Digest {
			continue
		}
		kept = append(kept, *a)
	}
	e.lock.Approval = kept
}

// checkNote refuses a note that carries a secret: the note is committed.
func (e *approveEnv) checkNote() error { return e.checkText("--note", approveNote) }

// checkText refuses free text that carries a secret: it is committed to the lock.
func (e *approveEnv) checkText(flag, text string) error {
	if text == "" {
		return nil
	}
	for _, f := range lint.ScanServed(e.cfg, "note", []lint.ServedFile{{Path: "note.txt", Content: []byte(text)}}, "") {
		if f.Code == lint.CodeSecretDetected {
			return oops.Errorf("%s looks like it contains a secret (%s); it would be committed to %s", flag, f.Code, lockfile.FileName)
		}
	}
	return nil
}

// confirm asks on a terminal; without --yes and without a terminal it refuses.
func (e *approveEnv) confirm(out io.Writer, n int, who string) error {
	if approveYes {
		return nil
	}
	if !stdinIsTerminal() {
		return oops.Hint("pass --yes to approve non-interactively").Errorf("not a terminal: refusing to approve without --yes")
	}
	if !askYesNo(fmt.Sprintf("Approve %d item(s) as %s? (y/N): ", n, safeText(who))) {
		fmt.Fprintln(out, "not approved") //nolint:errcheck // terminal output
		return oops.Errorf("not approved")
	}
	return nil
}

// stdinIsTerminal reports an interactive stdin: a character device that is not /dev/null.
func stdinIsTerminal() bool {
	info, err := os.Stdin.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	null, err := os.Stat(os.DevNull)
	return err != nil || !os.SameFile(info, null)
}

// recheck recomputes the digests right before writing: what was shown must be
// what is recorded, even if a file changed while the reviewer read it.
func (e *approveEnv) recheck(subs []approval.Subject) error {
	snap, err := lockSnapshot(cmdContext(), e.cfg, e.lock.Profile, true)
	if err != nil {
		return err
	}
	now := approval.SubjectsOf(e.lock, snap.Items)
	for _, s := range subs {
		cur, resolveErr := approval.Resolve(now, s.Ref())
		if resolveErr != nil || cur.Digest != s.Digest {
			return oops.Hint("review it again, then re-run approve").Errorf("%s changed while it was being reviewed; nothing was written", safeText(s.Ref()))
		}
	}
	return nil
}
