package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/samber/oops"
	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/approval"
	"github.com/Goldziher/ai-rulez/v5/internal/forge"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

var (
	verifyApprovals bool
	verifyOnline    bool
)

// verifyApprovalsReportVersion versions the JSON of `verify --approvals --format json`.
const verifyApprovalsReportVersion = 1

// Statuses of one checked approval record.
const (
	approvalCheckValid     = "valid"
	approvalCheckInvalid   = "invalid"
	approvalCheckUnchecked = "unchecked"
)

// approvalCheckReport is the JSON of `verify --approvals` (schema/verify-approvals.schema.json).
type approvalCheckReport struct {
	SchemaVersion int                   `json:"schema_version"`
	Online        bool                  `json:"online"`
	Results       []approvalCheckResult `json:"results"`
}

type approvalCheckResult struct {
	Ref       string `json:"ref"`
	Reviewer  string `json:"reviewer"`
	Assurance string `json:"assurance"`
	Status    string `json:"status"`
	Code      string `json:"code,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// rejectApprovalFlags fails when --online is used without --approvals, and when
// --approvals is combined with the modes it does not share flags with.
func rejectApprovalFlags(cmd *cobra.Command) error {
	if !verifyApprovals {
		if verifyOnline {
			return oops.Errorf("--online applies to --approvals")
		}
		return nil
	}
	if verifyAttestation || verifyArtifactMode() || verifyPlugin || verifyIfConfigured || verifyIfGenerated || verifyRecursive {
		return oops.Errorf("--approvals cannot be combined with --attestation, --bundle, --skill, --sbom, --plugin, --if-configured, --if-generated or --recursive")
	}
	for _, name := range verifyAttestationOnly {
		if cmd.Flags().Changed(name) {
			return oops.Errorf("--%s applies to --attestation", name)
		}
	}
	return checkFormatFlag(verifyFormat)
}

// runVerifyApprovals re-checks the approvals that apply to the current content:
// every signed approval is verified against its attestation and the
// [[signing.trust]] entries for approvals, offline; with --online, every
// review-linked approval is checked against the forge (the review still exists,
// still approves, and was made on this content). Records of content that has
// changed are not checked: they no longer apply. Exit codes: 0 every checked
// approval holds, 1 the check could not run, 2 an approval failed.
func runVerifyApprovals(args []string, out io.Writer) int {
	path := ""
	if len(args) > 0 {
		path = args[0]
	}
	env, err := loadApproveEnvAt(path)
	if err != nil {
		fmtError(err)
		return 1
	}
	results, err := env.checkApprovals(cmdContext(), verifyOnline)
	if err != nil {
		fmtError(err)
		return 1
	}
	return reportApprovalChecks(out, results, verifyOnline)
}

func (e *approveEnv) checkApprovals(ctx context.Context, online bool) ([]approvalCheckResult, error) {
	current := map[string]approval.Subject{}
	for _, s := range e.subjects {
		current[s.Key()] = s
	}
	var forgeSide *onlineCheck
	var results []approvalCheckResult
	for i := range e.lock.Approval {
		a := e.lock.Approval[i]
		s, ok := current[a.ItemKey()]
		if !ok || s.Digest != a.Digest || a.Assurance == lockfile.AssuranceAsserted {
			continue
		}
		r := approvalCheckResult{Ref: s.Ref(), Reviewer: a.Reviewer, Assurance: a.Assurance, Status: approvalCheckValid}
		if a.Assurance == lockfile.AssuranceReviewLinked && online && forgeSide == nil {
			var err error
			if forgeSide, err = e.newOnlineCheck(ctx); err != nil {
				return nil, err
			}
		}
		if err := e.checkOne(ctx, &r, a, s, online, forgeSide); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, nil
}

// checkOne fills r for one record. A failure to use the forge is returned (the
// check could not run); a record that does not hold is r.Status = invalid.
func (e *approveEnv) checkOne(ctx context.Context, r *approvalCheckResult, a lockfile.Approval, s approval.Subject, online bool, forgeSide *onlineCheck) error {
	invalid := func(err error) {
		r.Status, r.Code, r.Reason = approvalCheckInvalid, approval.CodeUnverified, err.Error()
	}
	switch a.Assurance {
	case lockfile.AssuranceSigned:
		if _, err := e.policy.VerifyAssurance(a, s, e.now); err != nil {
			invalid(err)
		}
	case lockfile.AssuranceReviewLinked:
		if !online {
			r.Status, r.Reason = approvalCheckUnchecked, "needs --online: a review link cannot be verified offline"
			return nil
		}
		if err := forgeSide.verify(ctx, a, s); err != nil {
			if forgeUnavailable(err) {
				return oops.Wrapf(err, "cannot check %s against the forge", s.Ref())
			}
			invalid(err)
		}
	default:
		invalid(fmt.Errorf("unknown assurance %q", a.Assurance))
	}
	return nil
}

// onlineCheck is the forge side of the re-check: the repository and the
// history reader that tells what the lock pinned at a reviewed commit.
type onlineCheck struct {
	repo forge.Repo
	git  *approveGit
	env  *approveEnv
}

func (e *approveEnv) newOnlineCheck(ctx context.Context) (*onlineCheck, error) {
	repo, err := originRepo(ctx, e)
	if err != nil {
		return nil, err
	}
	g, err := newApproveGit(ctx, e.cfg)
	if err != nil {
		return nil, err
	}
	return &onlineCheck{repo: repo, git: g, env: e}, nil
}

func (o *onlineCheck) verify(ctx context.Context, a lockfile.Approval, s approval.Subject) error {
	return approval.VerifyReviewRecord(ctx, approveForge(), a.Reviewer, a.Ref,
		approval.ReviewQuery{Repo: o.repo, Digest: s.Digest, PinnedAt: o.git.pinnedAt(s), Named: o.env.namedFor(s)})
}

// forgeUnavailable reports a failure to reach or use the forge, as opposed to a
// review that does not hold: the check could not run, which is exit 1, not a
// finding.
func forgeUnavailable(err error) bool {
	for _, target := range []error{forge.ErrOffline, forge.ErrUnauthorized, forge.ErrForbidden, forge.ErrRateLimited,
		forge.ErrTruncated, forge.ErrTooLarge, forge.ErrHostNotAllowed, forge.ErrUnsupportedSource} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

func reportApprovalChecks(out io.Writer, results []approvalCheckResult, online bool) int {
	failed := 0
	for _, r := range results {
		if r.Status == approvalCheckInvalid {
			failed++
		}
	}
	code := 0
	if failed > 0 {
		code = exitDrift
	}
	if verifyFormat == formatJSON {
		if results == nil {
			results = []approvalCheckResult{}
		}
		if err := writeRawJSON(out, approvalCheckReport{SchemaVersion: verifyApprovalsReportVersion, Online: online, Results: results}); err != nil {
			fmtError(oops.Wrapf(err, "write the report"))
			return 1
		}
		return code
	}
	if len(results) == 0 {
		fmt.Fprintln(out, "no approval above the asserted level applies to the current content") //nolint:errcheck // terminal output
		return code
	}
	for _, r := range results {
		switch r.Status {
		case approvalCheckValid:
			fmt.Fprintf(out, "OK    %-13s %s  %s\n", r.Assurance, safeText(r.Ref), safeText(r.Reviewer)) //nolint:errcheck // terminal output
		case approvalCheckUnchecked:
			fmt.Fprintf(out, "SKIP  %-13s %s  %s: %s\n", r.Assurance, safeText(r.Ref), safeText(r.Reviewer), r.Reason) //nolint:errcheck // terminal output
		default:
			fmt.Fprintf(os.Stderr, "FAIL  %-13s %s  %s  %s: %s\n", r.Assurance, safeText(r.Ref), safeText(r.Reviewer), r.Code, safeText(strings.TrimSpace(r.Reason)))
		}
	}
	return code
}
