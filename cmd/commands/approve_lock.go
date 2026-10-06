package commands

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/approval"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/govview"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

// carryApprovals keeps the approvals of the lock being replaced: `lock` re-pins
// content but never forgets who reviewed it. A full refresh drops, with a
// warning, the approvals of content that no longer exists (AR715); a refresh
// limited to some sources keeps every record. Records of changed content stay,
// stale, until the content is approved again or `approve --prune` removes them.
func carryApprovals(current, next *lockfile.File, full bool) {
	if current != nil {
		next.Deny = current.Deny // a deny entry outlives the content it names
	}
	if current == nil || len(current.Approval) == 0 {
		return
	}
	next.Approval = current.Approval
	if !full {
		return
	}
	subjects := approval.SubjectsOf(next, next.Item)
	orphans := approval.Orphans(current.Approval, subjects)
	if len(orphans) == 0 {
		return
	}
	gone := map[string]bool{}
	for _, a := range orphans {
		gone[a.ItemKey()] = true
		logger.Warn("Dropped an approval of content that no longer exists", "code", approval.CodeOrphan, "kind", a.Kind, "id", a.ID, "reviewer", a.Reviewer)
	}
	kept := next.Approval[:0:0]
	for _, a := range next.Approval {
		if !gone[a.ItemKey()] {
			kept = append(kept, a)
		}
	}
	next.Approval = kept
}

// approvalFindingsFor returns the AR710 to AR715 findings for strict validation,
// and AR716 when --approvals-base names a revision.
// It reports nothing without a lock, and nothing for a project that sets no
// [governance] policy and has no approval records. A policy that cannot be
// evaluated is itself a finding: enforcement never fails open.
func approvalFindingsFor(cfg *config.Config) []lint.ApprovalFinding {
	out := approvalStatusFindings(cfg)
	if validateApprovalsBase == "" {
		return out
	}
	lockRel := filepath.ToSlash(filepath.Join(relToBase(cfg, cfg.ConfigDir), lockfile.FileName))
	lock, err := lockfile.Load(cfg.ConfigDir)
	if err != nil {
		return append(out, lint.ApprovalFinding{Code: approval.CodeSelf, Path: lockRel, Message: fmt.Sprintf("cannot compare approvals with %q: %v", validateApprovalsBase, err)})
	}
	return append(out, selfApprovalFindings(cfg, lock, validateApprovalsBase, lockRel)...)
}

func approvalStatusFindings(cfg *config.Config) []lint.ApprovalFinding {
	policy := approval.PolicyOf(cfg)
	lock, err := lockfile.Load(cfg.ConfigDir)
	lockRel := filepath.ToSlash(filepath.Join(relToBase(cfg, cfg.ConfigDir), lockfile.FileName))
	unverifiable := func(err error) []lint.ApprovalFinding {
		return []lint.ApprovalFinding{{Code: approval.CodeMissing, Path: lockRel, Message: fmt.Sprintf("cannot verify approvals: %v", err)}}
	}
	if err != nil {
		if policy.Active() {
			return unverifiable(err)
		}
		return nil
	}
	if msg := policy.LockProblem(lock); msg != "" {
		return []lint.ApprovalFinding{{Code: approval.CodeMissing, Path: lockRel, Message: msg}}
	}
	if lock == nil || (!policy.Active() && len(lock.Approval) == 0 && len(lock.Deny) == 0) {
		return nil
	}
	shared, err := sharedConfig(cfg)
	if err == nil {
		var snapItems []lockfile.Item
		if lock.HasContentPins() || len(lock.Approval) > 0 {
			snap, snapErr := lockSnapshot(shared, lock.Profile, true)
			if snapErr != nil {
				return unverifiable(snapErr)
			}
			snapItems = snap.Items
		}
		st := govview.EvaluateApprovals(shared, lock, snapItems, govview.ApprovalNow())
		var out []lint.ApprovalFinding
		for _, r := range approval.Failures(st.Results) {
			out = append(out, lint.ApprovalFinding{Code: approval.CodeOf(r.Status), Path: lockRel, Message: r.Message()})
		}
		for _, a := range st.Orphans {
			out = append(out, lint.ApprovalFinding{Code: approval.CodeOrphan, Path: lockRel,
				Message: fmt.Sprintf("the approval by %s names %s:%s, which no longer exists; run `ai-rulez approve --prune`", a.Reviewer, a.Kind, a.ID)})
		}
		return append(out, unresolvedFindings(st.Policy, st.Subjects, lockRel)...)
	}
	if policy.Active() {
		return unverifiable(err)
	}
	return nil
}

// approvalLockedLines are the lines `generate --locked` adds to the drift it
// reports: content that needs approval under an enforced [governance] policy and
// lacks it. items are the current digests (what verifyLockedSources computed).
func approvalLockedLines(cfg *config.Config, lock *lockfile.File, items []lockfile.Item) []string {
	changes, _ := govview.ApprovalChanges(cfg, lock, items, govview.ApprovalNow())
	lines := make([]string, 0, len(changes))
	for i := range changes {
		lines = append(lines, changes[i].Line())
	}
	return lines
}

// unresolvedFindings is AR719: an approvers_from that cannot be read, and teams
// among the approvers or CODEOWNERS owners that have no member list. Both make
// authorization fail closed, so they are reported instead of left as a quiet
// "unauthorized". Nothing is reported unless the policy requires approvals.
func unresolvedFindings(policy approval.Policy, subjects []approval.Subject, lockRel string) []lint.ApprovalFinding {
	if !policy.Active() {
		return nil
	}
	var out []lint.ApprovalFinding
	if problem := policy.OwnersProblem(); problem != "" {
		out = append(out, lint.ApprovalFinding{Code: approval.CodeUnresolved, Path: lockRel, Message: problem})
	}
	if teams := policy.UnresolvedTeams(subjects); len(teams) > 0 {
		out = append(out, lint.ApprovalFinding{Code: approval.CodeUnresolved, Path: lockRel,
			Message: fmt.Sprintf("no members are known for %s: list them in [governance.teams], or resolve them with `ai-rulez approve --resolve-teams`; until then nobody is authorized through them", strings.Join(teams, ", "))})
	}
	return out
}

// deniedPinsError is the refusal of `lock` to write a lock that pins content on
// the deny list (AR717): a denied digest can be neither pinned nor approved.
func deniedPinsError(lock *lockfile.File) error {
	deny := lock.DenySet()
	if len(deny) == 0 {
		return nil
	}
	var refs []string
	for _, s := range approval.SubjectsOf(lock, lock.Item) {
		if reason, denied := deny[s.Digest]; denied {
			refs = append(refs, s.Ref()+reasonSuffix(reason))
		}
	}
	if len(refs) == 0 {
		return nil
	}
	return oops.Hint("remove or replace the content; `ai-rulez approve --revoke <item> --deny` adds entries").
		Errorf("%s refusing to pin content on the deny list: %s", approval.CodeDenied, strings.Join(refs, "; "))
}

func reasonSuffix(reason string) string {
	if reason == "" {
		return ""
	}
	return " (" + reason + ")"
}
